package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport"
	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/services"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// inboxUploadBodyMargin is the size added to the biggest file allowed to get the max size of the body of an upload (the other parts of the multipart)
const inboxUploadBodyMargin = 1024 * 1024

// InboxItemsApi represents the inbox items api
type InboxItemsApi struct {
	ApiUsingConfig
	inboxItems *services.InboxItemService
}

// Initialize an inbox items api singleton instance
var (
	InboxItems = &InboxItemsApi{
		ApiUsingConfig: ApiUsingConfig{
			container: settings.Container,
		},
		inboxItems: services.InboxItems,
	}
)

// InboxItemUploadHandler saves a receipt (image) or a statement (document) uploaded by current user as a pending inbox item
func (a *InboxItemsApi) InboxItemUploadHandler(c *core.WebContext) (any, *errs.Error) {
	uid := c.GetCurrentUid()

	if !a.CurrentConfig().EnableTransactionPictures {
		return nil, errs.ErrInboxItemStorageDisabled
	}

	// the limit is imposed on the whole body before parsing the multipart, so a big upload is rejected without being processed
	maxBodySize := a.getMaxUploadBodySize()

	if c.Request.ContentLength > maxBodySize {
		log.Warnf(c, "[inbox_items.InboxItemUploadHandler] the request body size \"%d\" exceeds the maximum size \"%d\" for user \"uid:%d\"", c.Request.ContentLength, maxBodySize, uid)
		return nil, errs.ErrExceedMaxInboxItemFileSize
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodySize)
	form, err := c.MultipartForm()

	if err != nil {
		var maxBytesError *http.MaxBytesError

		if errors.As(err, &maxBytesError) {
			log.Warnf(c, "[inbox_items.InboxItemUploadHandler] the request body exceeds the maximum size \"%d\" for user \"uid:%d\"", maxBodySize, uid)
			return nil, errs.ErrExceedMaxInboxItemFileSize
		}

		log.Errorf(c, "[inbox_items.InboxItemUploadHandler] failed to get multi-part form data for user \"uid:%d\", because %s", uid, err.Error())
		return nil, errs.ErrParameterInvalid
	}

	fileCount := 0

	for _, fileHeaders := range form.File {
		fileCount += len(fileHeaders)
	}

	if fileCount > 1 {
		log.Warnf(c, "[inbox_items.InboxItemUploadHandler] there are %d files in request, only one is allowed for user \"uid:%d\"", fileCount, uid)
		return nil, errs.ErrParameterInvalid
	}

	kindValues := form.Value["kind"]
	kindNumber := int64(0)

	if len(kindValues) > 0 {
		kindNumber, _ = utils.StringToInt64(kindValues[0])
	}

	kind := models.InboxItemKind(kindNumber)

	if kind != models.INBOX_ITEM_KIND_RECEIPT && kind != models.INBOX_ITEM_KIND_STATEMENT {
		return nil, errs.ErrInboxItemKindInvalid
	}

	files := form.File["file"]

	if len(files) < 1 {
		log.Warnf(c, "[inbox_items.InboxItemUploadHandler] there is no file in request for user \"uid:%d\"", uid)
		return nil, errs.ErrNoInboxItemFile
	}

	fileHeader := files[0]

	if fileHeader.Size < 1 {
		return nil, errs.ErrInboxItemFileIsEmpty
	}

	fileExtension := utils.GetFileNameExtension(fileHeader.Filename)

	if kind == models.INBOX_ITEM_KIND_RECEIPT {
		if utils.GetImageContentType(fileExtension) == "" {
			log.Warnf(c, "[inbox_items.InboxItemUploadHandler] the file extension \"%s\" of receipt in request is not supported for user \"uid:%d\"", fileExtension, uid)
			return nil, errs.ErrImageTypeNotSupported
		}

		if fileHeader.Size > int64(a.CurrentConfig().MaxTransactionPictureFileSize) {
			log.Warnf(c, "[inbox_items.InboxItemUploadHandler] the upload file size \"%d\" exceeds the maximum size \"%d\" of receipt for user \"uid:%d\"", fileHeader.Size, a.CurrentConfig().MaxTransactionPictureFileSize, uid)
			return nil, errs.ErrExceedMaxTransactionPictureFileSize
		}
	} else {
		if _, exists := models.InboxStatementFileExtensions[fileExtension]; !exists {
			log.Warnf(c, "[inbox_items.InboxItemUploadHandler] the file extension \"%s\" of statement in request is not supported for user \"uid:%d\"", fileExtension, uid)
			return nil, errs.ErrInboxItemFileTypeNotAllowed
		}

		if fileHeader.Size > models.InboxItemStatementMaxFileSize {
			log.Warnf(c, "[inbox_items.InboxItemUploadHandler] the upload file size \"%d\" exceeds the maximum size \"%d\" of statement for user \"uid:%d\"", fileHeader.Size, models.InboxItemStatementMaxFileSize, uid)
			return nil, errs.ErrExceedMaxInboxItemFileSize
		}
	}

	file, err := fileHeader.Open()

	if err != nil {
		log.Errorf(c, "[inbox_items.InboxItemUploadHandler] failed to get file from request for user \"uid:%d\", because %s", uid, err.Error())
		return nil, errs.ErrOperationFailed
	}

	note := ""

	if noteValues := form.Value["note"]; len(noteValues) > 0 {
		note = noteValues[0]
	}

	item := &models.InboxItem{
		Uid:              uid,
		FileExtension:    fileExtension,
		OriginalFileName: fileHeader.Filename,
		Note:             note,
		CreatedIp:        c.ClientIP(),
	}

	if kind == models.INBOX_ITEM_KIND_RECEIPT {
		err = a.inboxItems.CreateReceiptItem(c, item, file)
	} else {
		err = a.inboxItems.CreateStatementItem(c, item, file)
	}

	if err != nil {
		log.Errorf(c, "[inbox_items.InboxItemUploadHandler] failed to save inbox item for user \"uid:%d\", because %s", uid, err.Error())
		return nil, errs.Or(err, errs.ErrOperationFailed)
	}

	return a.getInboxItemInfoResponse(c, item)
}

// getMaxUploadBodySize returns the max size of the body of an upload: the biggest file allowed plus 1 MB for the other parts of the multipart
func (a *InboxItemsApi) getMaxUploadBodySize() int64 {
	maxFileSize := int64(models.InboxItemStatementMaxFileSize)

	if receiptMaxFileSize := int64(a.CurrentConfig().MaxTransactionPictureFileSize); receiptMaxFileSize > maxFileSize {
		maxFileSize = receiptMaxFileSize
	}

	return maxFileSize + inboxUploadBodyMargin
}

// InboxItemListHandler returns the inbox items of current user
func (a *InboxItemsApi) InboxItemListHandler(c *core.WebContext) (any, *errs.Error) {
	var listReq models.InboxItemListRequest
	err := c.ShouldBindQuery(&listReq)

	if err != nil {
		log.Warnf(c, "[inbox_items.InboxItemListHandler] parse request failed, because %s", err.Error())
		return nil, errs.NewIncompleteOrIncorrectSubmissionError(err)
	}

	uid := c.GetCurrentUid()
	items, totalCount, err := a.inboxItems.GetInboxItems(c, uid, listReq.Status, listReq.Kind, listReq.Page, listReq.Count)

	if err != nil {
		log.Errorf(c, "[inbox_items.InboxItemListHandler] failed to get inbox items for user \"uid:%d\", because %s", uid, err.Error())
		return nil, errs.Or(err, errs.ErrOperationFailed)
	}

	pictureToken, err := TransactionPictures.GetTransactionPictureToken(c, uid)

	if err != nil {
		return nil, errs.Or(err, errs.ErrTokenGenerating)
	}

	response := &models.InboxItemListResponse{
		Items:      make([]*models.InboxItemInfoResponse, len(items)),
		TotalCount: totalCount,
	}

	for i, item := range items {
		response.Items[i] = item.ToInboxItemInfoResponse(a.getPictureUrl(item, pictureToken))
	}

	return response, nil
}

// InboxItemGetHandler returns one specific inbox item of current user
func (a *InboxItemsApi) InboxItemGetHandler(c *core.WebContext) (any, *errs.Error) {
	var getReq models.InboxItemGetRequest
	err := c.ShouldBindQuery(&getReq)

	if err != nil {
		log.Warnf(c, "[inbox_items.InboxItemGetHandler] parse request failed, because %s", err.Error())
		return nil, errs.NewIncompleteOrIncorrectSubmissionError(err)
	}

	uid := c.GetCurrentUid()
	item, err := a.inboxItems.GetInboxItemByItemId(c, uid, getReq.Id)

	if err != nil {
		log.Errorf(c, "[inbox_items.InboxItemGetHandler] failed to get inbox item \"id:%d\" for user \"uid:%d\", because %s", getReq.Id, uid, err.Error())
		return nil, errs.Or(err, errs.ErrOperationFailed)
	}

	return a.getInboxItemInfoResponse(c, item)
}

// InboxItemStatsHandler returns the count of pending receipts and statements of current user
func (a *InboxItemsApi) InboxItemStatsHandler(c *core.WebContext) (any, *errs.Error) {
	uid := c.GetCurrentUid()
	receipts, statements, err := a.inboxItems.GetPendingCounts(c, uid)

	if err != nil {
		log.Errorf(c, "[inbox_items.InboxItemStatsHandler] failed to get pending counts for user \"uid:%d\", because %s", uid, err.Error())
		return nil, errs.Or(err, errs.ErrOperationFailed)
	}

	return &models.InboxItemStatsResponse{
		PendingReceipts:   receipts,
		PendingStatements: statements,
	}, nil
}

// InboxItemDiscardHandler discards a pending inbox item of current user
func (a *InboxItemsApi) InboxItemDiscardHandler(c *core.WebContext) (any, *errs.Error) {
	var discardReq models.InboxItemDiscardRequest
	err := c.ShouldBindJSON(&discardReq)

	if err != nil {
		log.Warnf(c, "[inbox_items.InboxItemDiscardHandler] parse request failed, because %s", err.Error())
		return nil, errs.NewIncompleteOrIncorrectSubmissionError(err)
	}

	uid := c.GetCurrentUid()
	err = a.inboxItems.DiscardInboxItem(c, uid, discardReq.Id, discardReq.Note)

	if err != nil {
		log.Errorf(c, "[inbox_items.InboxItemDiscardHandler] failed to discard inbox item \"id:%d\" for user \"uid:%d\", because %s", discardReq.Id, uid, err.Error())
		return nil, errs.Or(err, errs.ErrOperationFailed)
	}

	return true, nil
}

// InboxItemReopenHandler sets a processed or discarded inbox item of current user back to pending
func (a *InboxItemsApi) InboxItemReopenHandler(c *core.WebContext) (any, *errs.Error) {
	var reopenReq models.InboxItemReopenRequest
	err := c.ShouldBindJSON(&reopenReq)

	if err != nil {
		log.Warnf(c, "[inbox_items.InboxItemReopenHandler] parse request failed, because %s", err.Error())
		return nil, errs.NewIncompleteOrIncorrectSubmissionError(err)
	}

	uid := c.GetCurrentUid()
	err = a.inboxItems.ReopenInboxItem(c, uid, reopenReq.Id)

	if err != nil {
		log.Errorf(c, "[inbox_items.InboxItemReopenHandler] failed to reopen inbox item \"id:%d\" for user \"uid:%d\", because %s", reopenReq.Id, uid, err.Error())
		return nil, errs.Or(err, errs.ErrOperationFailed)
	}

	return true, nil
}

// InboxItemDeleteHandler deletes a pending or discarded inbox item of current user
func (a *InboxItemsApi) InboxItemDeleteHandler(c *core.WebContext) (any, *errs.Error) {
	var deleteReq models.InboxItemDeleteRequest
	err := c.ShouldBindJSON(&deleteReq)

	if err != nil {
		log.Warnf(c, "[inbox_items.InboxItemDeleteHandler] parse request failed, because %s", err.Error())
		return nil, errs.NewIncompleteOrIncorrectSubmissionError(err)
	}

	uid := c.GetCurrentUid()
	err = a.inboxItems.DeleteInboxItem(c, uid, deleteReq.Id)

	if err != nil {
		log.Errorf(c, "[inbox_items.InboxItemDeleteHandler] failed to delete inbox item \"id:%d\" for user \"uid:%d\", because %s", deleteReq.Id, uid, err.Error())
		return nil, errs.Or(err, errs.ErrOperationFailed)
	}

	return true, nil
}

// InboxItemFileGetHandler returns the file (receipt image or statement document) of an inbox item of current user
func (a *InboxItemsApi) InboxItemFileGetHandler(c *core.WebContext) ([]byte, string, *errs.Error) {
	itemId, err := utils.StringToInt64(c.Param("id"))

	if err != nil || itemId <= 0 {
		return nil, "", errs.ErrInboxItemIdInvalid
	}

	return a.readItemFile(c, c.GetCurrentUid(), itemId)
}

// InboxItemFileDownloadByTokenHandler returns the file of an inbox item to the holder of a single-use download token (created by the get_inbox_item tool)
func (a *InboxItemsApi) InboxItemFileDownloadByTokenHandler(c *core.WebContext) ([]byte, string, *errs.Error) {
	uid, itemId, err := aiimport.ParseDownloadToken(a.CurrentConfig().SecretKey, c.Param("token"), time.Now())

	if err != nil {
		return nil, "", errs.ErrInboxDownloadTokenInvalid
	}

	return a.readItemFile(c, uid, itemId)
}

func (a *InboxItemsApi) readItemFile(c *core.WebContext, uid int64, itemId int64) ([]byte, string, *errs.Error) {
	item, err := a.inboxItems.GetInboxItemByItemId(c, uid, itemId)

	if err != nil {
		return nil, "", errs.Or(err, errs.ErrOperationFailed)
	}

	data, contentType, err := a.inboxItems.ReadInboxItemFile(c, item)

	if err != nil {
		if !errs.IsCustomError(err) {
			log.Errorf(c, "[inbox_items.readItemFile] failed to read the file of inbox item \"id:%d\" for user \"uid:%d\", because %s", itemId, uid, err.Error())
		}

		return nil, "", errs.Or(err, errs.ErrOperationFailed)
	}

	return data, contentType, nil
}

func (a *InboxItemsApi) getInboxItemInfoResponse(c *core.WebContext, item *models.InboxItem) (*models.InboxItemInfoResponse, *errs.Error) {
	pictureToken, err := TransactionPictures.GetTransactionPictureToken(c, item.Uid)

	if err != nil {
		return nil, errs.Or(err, errs.ErrTokenGenerating)
	}

	return item.ToInboxItemInfoResponse(a.getPictureUrl(item, pictureToken)), nil
}

func (a *InboxItemsApi) getPictureUrl(item *models.InboxItem, pictureToken string) string {
	if item.Kind != models.INBOX_ITEM_KIND_RECEIPT || item.PictureId <= 0 {
		return ""
	}

	pictureInfo := &models.TransactionPictureInfo{
		PictureId:        item.PictureId,
		PictureExtension: item.FileExtension,
	}

	return a.GetTransactionPictureInfoResponse(pictureInfo, pictureToken).OriginalUrl
}
