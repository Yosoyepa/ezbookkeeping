package services

import (
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"time"

	"xorm.io/xorm"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/datastore"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/storage"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/mayswind/ezbookkeeping/pkg/uuid"
)

const inboxDocumentsPathPrefix = "inbox_documents"

// InboxItemService represents the inbox (receipts and statements waiting to be processed) service
type InboxItemService struct {
	ServiceUsingDB
	ServiceUsingUuid
	ServiceUsingStorage
}

// Initialize an inbox item service singleton instance
var (
	InboxItems = &InboxItemService{
		ServiceUsingDB: ServiceUsingDB{
			container: datastore.Container,
		},
		ServiceUsingUuid: ServiceUsingUuid{
			container: uuid.Container,
		},
		ServiceUsingStorage: ServiceUsingStorage{
			container: storage.Container,
		},
	}
)

// GetInboxItemByItemId returns an inbox item model according to inbox item id
func (s *InboxItemService) GetInboxItemByItemId(c core.Context, uid int64, itemId int64) (*models.InboxItem, error) {
	if uid <= 0 {
		return nil, errs.ErrUserIdInvalid
	}

	if itemId <= 0 {
		return nil, errs.ErrInboxItemIdInvalid
	}

	item := &models.InboxItem{}
	has, err := s.UserDataDB(uid).NewSession(c).ID(itemId).Where("uid=? AND deleted=?", uid, false).Get(item)

	if err != nil {
		return nil, err
	} else if !has {
		return nil, errs.ErrInboxItemNotFound
	}

	return item, nil
}

// GetInboxItems returns inbox item models (newest first) and the total count which match the filters
// status or kind equal to 0 means all
func (s *InboxItemService) GetInboxItems(c core.Context, uid int64, status models.InboxItemStatus, kind models.InboxItemKind, page int32, count int32) ([]*models.InboxItem, int64, error) {
	if uid <= 0 {
		return nil, 0, errs.ErrUserIdInvalid
	}

	if page < 1 {
		page = 1
	}

	if count < 1 {
		count = 20
	}

	newQuery := func() *xorm.Session {
		sess := s.UserDataDB(uid).NewSession(c).Where("uid=? AND deleted=?", uid, false)

		if status > 0 {
			sess = sess.And("status=?", status)
		}

		if kind > 0 {
			sess = sess.And("kind=?", kind)
		}

		return sess
	}

	totalCount, err := newQuery().Count(&models.InboxItem{})

	if err != nil {
		return nil, 0, err
	}

	var items []*models.InboxItem
	err = newQuery().OrderBy("created_unix_time desc, item_id desc").Limit(int(count), int((page-1)*count)).Find(&items)

	if err != nil {
		return nil, 0, err
	}

	return items, totalCount, nil
}

// GetPendingCounts returns the count of pending receipts and pending statements
func (s *InboxItemService) GetPendingCounts(c core.Context, uid int64) (int64, int64, error) {
	if uid <= 0 {
		return 0, 0, errs.ErrUserIdInvalid
	}

	receipts, err := s.UserDataDB(uid).NewSession(c).Where("uid=? AND deleted=? AND status=? AND kind=?", uid, false, models.INBOX_ITEM_STATUS_PENDING, models.INBOX_ITEM_KIND_RECEIPT).Count(&models.InboxItem{})

	if err != nil {
		return 0, 0, err
	}

	statements, err := s.UserDataDB(uid).NewSession(c).Where("uid=? AND deleted=? AND status=? AND kind=?", uid, false, models.INBOX_ITEM_STATUS_PENDING, models.INBOX_ITEM_KIND_STATEMENT).Count(&models.InboxItem{})

	if err != nil {
		return 0, 0, err
	}

	return receipts, statements, nil
}

// CreateReceiptItem saves the receipt image as an unlinked transaction picture and creates a pending inbox item for it
func (s *InboxItemService) CreateReceiptItem(c core.Context, item *models.InboxItem, pictureFile multipart.File) error {
	if item.Uid <= 0 {
		return errs.ErrUserIdInvalid
	}

	itemId := s.GenerateUuid(uuid.UUID_TYPE_INBOX_ITEM)

	if itemId < 1 {
		pictureFile.Close()
		return errs.ErrSystemIsBusy
	}

	pictureInfo := &models.TransactionPictureInfo{
		Uid:              item.Uid,
		TransactionId:    models.TransactionPictureNewPictureTransactionId,
		PictureExtension: item.FileExtension,
		CreatedIp:        item.CreatedIp,
	}

	err := TransactionPictures.UploadPicture(c, pictureInfo, pictureFile)

	if err != nil {
		return err
	}

	item.ItemId = itemId
	item.Kind = models.INBOX_ITEM_KIND_RECEIPT
	item.PictureId = pictureInfo.PictureId

	err = s.insertNewItem(c, item)

	if err != nil {
		s.deleteReceiptPicture(c, item)
		return err
	}

	return nil
}

// CreateStatementItem saves the statement document into the object storage and creates a pending inbox item for it
func (s *InboxItemService) CreateStatementItem(c core.Context, item *models.InboxItem, document io.ReadSeekCloser) error {
	if item.Uid <= 0 {
		return errs.ErrUserIdInvalid
	}

	defer document.Close()

	if _, exists := models.InboxStatementFileExtensions[item.FileExtension]; !exists {
		return errs.ErrInboxItemFileTypeNotAllowed
	}

	itemId := s.GenerateUuid(uuid.UUID_TYPE_INBOX_ITEM)

	if itemId < 1 {
		return errs.ErrSystemIsBusy
	}

	item.ItemId = itemId
	item.Kind = models.INBOX_ITEM_KIND_STATEMENT
	item.PictureId = 0

	err := s.ServiceUsingStorage.container.SaveTransactionPicture(c, s.getInboxDocumentPath(item), document)

	if err != nil {
		return err
	}

	err = s.insertNewItem(c, item)

	if err != nil {
		_ = s.ServiceUsingStorage.container.DeleteTransactionPicture(c, s.getInboxDocumentPath(item))
		return err
	}

	return nil
}

// ReadInboxItemFile returns the content and the content type of the file (receipt image or statement document) of the inbox item
func (s *InboxItemService) ReadInboxItemFile(c core.Context, item *models.InboxItem) ([]byte, string, error) {
	if item.Kind == models.INBOX_ITEM_KIND_RECEIPT {
		data, err := TransactionPictures.GetPictureByPictureId(c, item.Uid, item.PictureId, item.FileExtension)

		if err != nil {
			return nil, "", err
		}

		return data, utils.GetImageContentType(item.FileExtension), nil
	}

	contentType, exists := models.InboxStatementFileExtensions[item.FileExtension]

	if !exists {
		return nil, "", errs.ErrInboxItemFileTypeNotAllowed
	}

	object, exists, err := s.ServiceUsingStorage.container.ReadTransactionPicture(c, s.getInboxDocumentPath(item))

	if err != nil {
		return nil, "", err
	} else if !exists {
		return nil, "", errs.ErrInboxItemFileNotExists
	}

	defer object.Close()

	data, err := io.ReadAll(object)

	if err != nil {
		return nil, "", err
	}

	return data, contentType, nil
}

// MarkInboxItemProcessed marks a pending inbox item as processed and saves the ids of the transactions created from it
func (s *InboxItemService) MarkInboxItemProcessed(c core.Context, uid int64, itemId int64, transactionIds []int64, processedNote string) error {
	now := time.Now().Unix()
	updateModel := &models.InboxItem{
		Status:               models.INBOX_ITEM_STATUS_PROCESSED,
		ResultTransactionIds: models.FormatInboxItemResultTransactionIds(transactionIds),
		ProcessedNote:        truncateRunes(processedNote, 1024),
		ProcessedUnixTime:    now,
		UpdatedUnixTime:      now,
	}

	return s.updateInboxItemStatus(c, uid, itemId, models.INBOX_ITEM_STATUS_PENDING, updateModel, "status", "result_transaction_ids", "processed_note", "processed_unix_time", "updated_unix_time")
}

// DiscardInboxItem marks a pending inbox item as discarded
func (s *InboxItemService) DiscardInboxItem(c core.Context, uid int64, itemId int64, reason string) error {
	now := time.Now().Unix()
	updateModel := &models.InboxItem{
		Status:            models.INBOX_ITEM_STATUS_DISCARDED,
		ProcessedNote:     truncateRunes(reason, 1024),
		ProcessedUnixTime: now,
		UpdatedUnixTime:   now,
	}

	return s.updateInboxItemStatus(c, uid, itemId, models.INBOX_ITEM_STATUS_PENDING, updateModel, "status", "processed_note", "processed_unix_time", "updated_unix_time")
}

// ReopenInboxItem sets a processed or discarded inbox item back to pending, it does not delete any transactions created before
func (s *InboxItemService) ReopenInboxItem(c core.Context, uid int64, itemId int64) error {
	if uid <= 0 {
		return errs.ErrUserIdInvalid
	}

	item, err := s.GetInboxItemByItemId(c, uid, itemId)

	if err != nil {
		return err
	}

	if item.Status == models.INBOX_ITEM_STATUS_PENDING {
		return errs.ErrInboxItemStatusInvalid
	}

	updateModel := &models.InboxItem{
		Status:          models.INBOX_ITEM_STATUS_PENDING,
		UpdatedUnixTime: time.Now().Unix(),
	}

	return s.updateInboxItemStatus(c, uid, itemId, item.Status, updateModel, "status", "updated_unix_time")
}

// DeleteInboxItem deletes a pending or discarded inbox item and its file
func (s *InboxItemService) DeleteInboxItem(c core.Context, uid int64, itemId int64) error {
	item, err := s.GetInboxItemByItemId(c, uid, itemId)

	if err != nil {
		return err
	}

	if item.Status != models.INBOX_ITEM_STATUS_PENDING && item.Status != models.INBOX_ITEM_STATUS_DISCARDED {
		return errs.ErrInboxItemCannotBeDeleted
	}

	now := time.Now().Unix()
	updateModel := &models.InboxItem{
		Deleted:         true,
		DeletedUnixTime: now,
		UpdatedUnixTime: now,
	}

	err = s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		updatedRows, err := sess.ID(itemId).Cols("deleted", "deleted_unix_time", "updated_unix_time").Where("uid=? AND deleted=? AND (status=? OR status=?)", uid, false, models.INBOX_ITEM_STATUS_PENDING, models.INBOX_ITEM_STATUS_DISCARDED).Update(updateModel)

		if err != nil {
			return err
		} else if updatedRows < 1 {
			return errs.ErrInboxItemCannotBeDeleted
		}

		return nil
	})

	if err != nil {
		return err
	}

	if item.Kind == models.INBOX_ITEM_KIND_RECEIPT {
		s.deleteReceiptPicture(c, item)
	} else {
		err = s.ServiceUsingStorage.container.DeleteTransactionPicture(c, s.getInboxDocumentPath(item))

		if err != nil {
			log.Warnf(c, "[inbox_items.DeleteInboxItem] failed to delete the file of inbox item \"id:%d\" for user \"uid:%d\", because %s", itemId, uid, err.Error())
		}
	}

	return nil
}

// GetNewReceiptPictureId returns the picture id of the receipt when it is not linked to any transaction yet, otherwise returns 0
func (s *InboxItemService) GetNewReceiptPictureId(c core.Context, item *models.InboxItem) (int64, error) {
	if item.Kind != models.INBOX_ITEM_KIND_RECEIPT || item.PictureId <= 0 {
		return 0, nil
	}

	pictureInfos, err := TransactionPictures.GetNewPictureInfosByPictureIds(c, item.Uid, []int64{item.PictureId})

	if err != nil {
		return 0, err
	}

	if len(pictureInfos) < 1 {
		return 0, nil
	}

	return item.PictureId, nil
}

// FindDuplicateCandidates returns the transactions of the account with the same type and amount whose time is in the specified range
func (s *InboxItemService) FindDuplicateCandidates(c core.Context, uid int64, accountId int64, transactionType models.TransactionDbType, amount int64, minTransactionTime int64, maxTransactionTime int64) ([]*models.Transaction, error) {
	if uid <= 0 {
		return nil, errs.ErrUserIdInvalid
	}

	var transactions []*models.Transaction
	err := s.UserDataDB(uid).NewSession(c).Where("uid=? AND deleted=? AND account_id=? AND type=? AND amount=? AND transaction_time>=? AND transaction_time<=?", uid, false, accountId, transactionType, amount, minTransactionTime, maxTransactionTime).OrderBy("transaction_time asc").Find(&transactions)

	return transactions, err
}

// GetTransactionsForSummary returns the transactions of the specified type in the time range
func (s *InboxItemService) GetTransactionsForSummary(c core.Context, uid int64, transactionType models.TransactionDbType, minTransactionTime int64, maxTransactionTime int64) ([]*models.Transaction, error) {
	if uid <= 0 {
		return nil, errs.ErrUserIdInvalid
	}

	var transactions []*models.Transaction
	err := s.UserDataDB(uid).NewSession(c).Where("uid=? AND deleted=? AND type=? AND transaction_time>=? AND transaction_time<=?", uid, false, transactionType, minTransactionTime, maxTransactionTime).OrderBy("transaction_time asc").Find(&transactions)

	return transactions, err
}

// GetTagIndexesForSummary returns the transaction tag indexes in the time range
func (s *InboxItemService) GetTagIndexesForSummary(c core.Context, uid int64, minTransactionTime int64, maxTransactionTime int64) ([]*models.TransactionTagIndex, error) {
	if uid <= 0 {
		return nil, errs.ErrUserIdInvalid
	}

	var tagIndexes []*models.TransactionTagIndex
	err := s.UserDataDB(uid).NewSession(c).Where("uid=? AND deleted=? AND transaction_time>=? AND transaction_time<=?", uid, false, minTransactionTime, maxTransactionTime).Find(&tagIndexes)

	return tagIndexes, err
}

func (s *InboxItemService) insertNewItem(c core.Context, item *models.InboxItem) error {
	now := time.Now().Unix()

	item.Status = models.INBOX_ITEM_STATUS_PENDING
	item.Deleted = false
	item.ResultTransactionIds = ""
	item.ProcessedNote = ""
	item.CreatedUnixTime = now
	item.UpdatedUnixTime = now
	item.OriginalFileName = truncateRunes(item.OriginalFileName, 255)
	item.Note = truncateRunes(item.Note, 255)

	return s.UserDataDB(item.Uid).DoTransaction(c, func(sess *xorm.Session) error {
		_, err := sess.Insert(item)
		return err
	})
}

func (s *InboxItemService) updateInboxItemStatus(c core.Context, uid int64, itemId int64, expectedStatus models.InboxItemStatus, updateModel *models.InboxItem, cols ...string) error {
	if uid <= 0 {
		return errs.ErrUserIdInvalid
	}

	if itemId <= 0 {
		return errs.ErrInboxItemIdInvalid
	}

	return s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		updatedRows, err := sess.ID(itemId).Cols(cols...).Where("uid=? AND deleted=? AND status=?", uid, false, expectedStatus).Update(updateModel)

		if err != nil {
			return err
		} else if updatedRows < 1 {
			exists, err := sess.Where("uid=? AND deleted=?", uid, false).And("item_id=?", itemId).Exist(&models.InboxItem{})

			if err != nil {
				return err
			} else if !exists {
				return errs.ErrInboxItemNotFound
			}

			return errs.ErrInboxItemNotPending
		}

		return nil
	})
}

// deleteReceiptPicture removes the picture of a receipt only if it is not linked to any transaction
func (s *InboxItemService) deleteReceiptPicture(c core.Context, item *models.InboxItem) {
	if item.PictureId <= 0 {
		return
	}

	now := time.Now().Unix()
	updateModel := &models.TransactionPictureInfo{
		Deleted:         true,
		DeletedUnixTime: now,
	}

	var updatedRows int64

	err := s.UserDataDB(item.Uid).DoTransaction(c, func(sess *xorm.Session) error {
		rows, err := sess.ID(item.PictureId).Cols("deleted", "deleted_unix_time").Where("uid=? AND deleted=? AND transaction_id=?", item.Uid, false, models.TransactionPictureNewPictureTransactionId).Update(updateModel)
		updatedRows = rows

		return err
	})

	if err != nil {
		log.Warnf(c, "[inbox_items.deleteReceiptPicture] failed to remove the picture \"id:%d\" of inbox item for user \"uid:%d\", because %s", item.PictureId, item.Uid, err.Error())
		return
	}

	if updatedRows > 0 {
		err = TransactionPictures.DeleteTransactionPicture(c, item.Uid, item.PictureId, item.FileExtension)

		if err != nil {
			log.Warnf(c, "[inbox_items.deleteReceiptPicture] failed to delete the picture file \"id:%d\" of inbox item for user \"uid:%d\", because %s", item.PictureId, item.Uid, err.Error())
		}
	}
}

func (s *InboxItemService) getInboxDocumentPath(item *models.InboxItem) string {
	return filepath.Join(inboxDocumentsPathPrefix, utils.Int64ToString(item.Uid), fmt.Sprintf("%d.%s", item.ItemId, item.FileExtension))
}

func truncateRunes(str string, maxRunes int) string {
	runes := []rune(str)

	if len(runes) <= maxRunes {
		return str
	}

	return string(runes[:maxRunes])
}
