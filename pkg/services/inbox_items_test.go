package services_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"xorm.io/xorm"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport/aitest"
	"github.com/mayswind/ezbookkeeping/pkg/datastore"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/services"
)

func TestInboxItems_UploadListAndStats(t *testing.T) {
	env := aitest.NewEnv(t)

	receipt := env.NewReceipt("recibo del Éxito")
	statement := env.NewStatement("csv", []byte("fecha;descripcion;valor\n15/10/2026;EXITO;-45.900\n"))
	pdf := env.NewStatement("pdf", []byte("%PDF-1.4 fake"))

	assert.Greater(t, receipt.ItemId, int64(0))
	assert.Equal(t, models.INBOX_ITEM_KIND_RECEIPT, receipt.Kind)
	assert.Greater(t, receipt.PictureId, int64(0))
	assert.Equal(t, models.INBOX_ITEM_STATUS_PENDING, receipt.Status)
	assert.Equal(t, models.INBOX_ITEM_KIND_STATEMENT, statement.Kind)
	assert.Equal(t, int64(0), statement.PictureId)

	receipts, statements, err := services.InboxItems.GetPendingCounts(env.Ctx, aitest.Uid)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), receipts)
	assert.Equal(t, int64(2), statements)

	items, total, err := services.InboxItems.GetInboxItems(env.Ctx, aitest.Uid, 0, 0, 1, 10)
	assert.Nil(t, err)
	assert.Equal(t, int64(3), total)
	assert.Equal(t, 3, len(items))

	items, total, err = services.InboxItems.GetInboxItems(env.Ctx, aitest.Uid, models.INBOX_ITEM_STATUS_PENDING, models.INBOX_ITEM_KIND_STATEMENT, 1, 10)
	assert.Nil(t, err)
	assert.Equal(t, int64(2), total)
	assert.ElementsMatch(t, []int64{statement.ItemId, pdf.ItemId}, []int64{items[0].ItemId, items[1].ItemId})

	// pagination
	items, total, err = services.InboxItems.GetInboxItems(env.Ctx, aitest.Uid, 0, 0, 2, 2)
	assert.Nil(t, err)
	assert.Equal(t, int64(3), total)
	assert.Equal(t, 1, len(items))

	// items of another user are not visible
	items, total, err = services.InboxItems.GetInboxItems(env.Ctx, 999, 0, 0, 1, 10)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, 0, len(items))

	_, err = services.InboxItems.GetInboxItemByItemId(env.Ctx, 999, receipt.ItemId)
	assert.Equal(t, errs.ErrInboxItemNotFound, err)
}

func TestInboxItems_ReadFiles(t *testing.T) {
	env := aitest.NewEnv(t)

	receipt := env.NewReceipt("")
	content := []byte("fecha;descripcion;valor\n15/10/2026;EXITO;-45.900\n")
	statement := env.NewStatement("csv", content)

	data, contentType, err := services.InboxItems.ReadInboxItemFile(env.Ctx, receipt)
	assert.Nil(t, err)
	assert.Equal(t, aitest.FakeJpeg, data)
	assert.Equal(t, "image/jpeg", contentType)

	data, contentType, err = services.InboxItems.ReadInboxItemFile(env.Ctx, statement)
	assert.Nil(t, err)
	assert.Equal(t, content, data)
	assert.Equal(t, "text/csv", contentType)
}

func TestInboxItems_DiscardReopenDelete(t *testing.T) {
	env := aitest.NewEnv(t)
	receipt := env.NewReceipt("")

	assert.Nil(t, services.InboxItems.DiscardInboxItem(env.Ctx, aitest.Uid, receipt.ItemId, "no es un recibo"))

	item, err := services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, receipt.ItemId)
	assert.Nil(t, err)
	assert.Equal(t, models.INBOX_ITEM_STATUS_DISCARDED, item.Status)
	assert.Equal(t, "no es un recibo", item.ProcessedNote)

	receipts, _, _ := services.InboxItems.GetPendingCounts(env.Ctx, aitest.Uid)
	assert.Equal(t, int64(0), receipts)

	// a discarded item cannot be discarded or processed again, it must be reopened first
	assert.Equal(t, errs.ErrInboxItemNotPending, services.InboxItems.DiscardInboxItem(env.Ctx, aitest.Uid, receipt.ItemId, "otra vez"))
	assert.Equal(t, errs.ErrInboxItemNotPending, services.InboxItems.MarkInboxItemProcessed(env.Ctx, aitest.Uid, receipt.ItemId, []int64{1}, ""))

	assert.Nil(t, services.InboxItems.ReopenInboxItem(env.Ctx, aitest.Uid, receipt.ItemId))
	item, _ = services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, receipt.ItemId)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PENDING, item.Status)

	// a pending item cannot be reopened
	assert.Equal(t, errs.ErrInboxItemStatusInvalid, services.InboxItems.ReopenInboxItem(env.Ctx, aitest.Uid, receipt.ItemId))

	// delete removes the item and the picture
	assert.Nil(t, services.InboxItems.DeleteInboxItem(env.Ctx, aitest.Uid, receipt.ItemId))
	_, err = services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, receipt.ItemId)
	assert.Equal(t, errs.ErrInboxItemNotFound, err)

	_, err = services.TransactionPictures.GetPictureInfoByPictureId(env.Ctx, aitest.Uid, receipt.PictureId)
	assert.Equal(t, errs.ErrTransactionPictureNotFound, err)

	assert.Equal(t, errs.ErrInboxItemNotFound, services.InboxItems.DeleteInboxItem(env.Ctx, aitest.Uid, receipt.ItemId))
}

func TestInboxItems_ProcessedItemCannotBeDeleted(t *testing.T) {
	env := aitest.NewEnv(t)
	statement := env.NewStatement("txt", []byte("hola"))

	assert.Nil(t, services.InboxItems.MarkInboxItemProcessed(env.Ctx, aitest.Uid, statement.ItemId, []int64{11, 22, 33}, strings.Repeat("x", 2000)))

	item, err := services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, statement.ItemId)
	assert.Nil(t, err)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PROCESSED, item.Status)
	assert.Equal(t, "11,22,33", item.ResultTransactionIds)
	assert.Equal(t, []int64{11, 22, 33}, item.GetResultTransactionIds())
	assert.Equal(t, 1024, len(item.ProcessedNote))
	assert.Greater(t, item.ProcessedUnixTime, int64(0))

	assert.Equal(t, errs.ErrInboxItemCannotBeDeleted, services.InboxItems.DeleteInboxItem(env.Ctx, aitest.Uid, statement.ItemId))

	// reopening does not delete the ids of the transactions created before
	assert.Nil(t, services.InboxItems.ReopenInboxItem(env.Ctx, aitest.Uid, statement.ItemId))
	item, _ = services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, statement.ItemId)
	assert.Equal(t, "11,22,33", item.ResultTransactionIds)
}

// H3: a statement can have 500 rows, so all the ids of the transactions must be kept
func TestInboxItems_KeepsAllTheResultTransactionIds(t *testing.T) {
	env := aitest.NewEnv(t)
	statement := env.NewStatement("csv", []byte("a;b\n"))

	ids := make([]int64, 500)

	for i := range ids {
		ids[i] = 1000000000000000 + int64(i)*7
	}

	assert.Nil(t, services.InboxItems.MarkInboxItemProcessed(env.Ctx, aitest.Uid, statement.ItemId, ids, "ok"))

	item, err := services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, statement.ItemId)
	assert.Nil(t, err)
	assert.Greater(t, len(item.ResultTransactionIds), 2048)
	assert.Equal(t, ids, item.GetResultTransactionIds())
	assert.Equal(t, 500, len(item.ToInboxItemInfoResponse("").ResultTransactionIds))
	assert.Equal(t, "1000000000000000", item.ToInboxItemInfoResponse("").ResultTransactionIds[0])
	assert.Equal(t, "1000000000003493", item.ToInboxItemInfoResponse("").ResultTransactionIds[499])
}

// H3: the column of a database created with the old model (VARCHAR(2048)) is synchronized with the new one (TEXT) and keeps all the ids.
// MySQL and PostgreSQL alter the column; SQLite does not enforce the length of VARCHAR, so it behaves like TEXT
func TestInboxItems_SyncsTheResultTransactionIdsColumnOfAnOldDatabase(t *testing.T) {
	env := aitest.NewEnv(t)

	type InboxItem struct {
		Uid                  int64                  `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created) NOT NULL"`
		Deleted              bool                   `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created) NOT NULL"`
		Status               models.InboxItemStatus `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created) NOT NULL"`
		ItemId               int64                  `xorm:"PK"`
		Kind                 models.InboxItemKind   `xorm:"NOT NULL"`
		PictureId            int64
		FileExtension        string `xorm:"VARCHAR(10) NOT NULL"`
		OriginalFileName     string `xorm:"VARCHAR(255)"`
		Note                 string `xorm:"VARCHAR(255)"`
		ResultTransactionIds string `xorm:"VARCHAR(2048)"`
		ProcessedNote        string `xorm:"VARCHAR(1024)"`
		CreatedIp            string `xorm:"VARCHAR(39)"`
		CreatedUnixTime      int64  `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created)"`
		UpdatedUnixTime      int64
		ProcessedUnixTime    int64
		DeletedUnixTime      int64
	}

	userDataStore := datastore.Container.UserDataStore

	err := userDataStore.Choose(aitest.Uid).DoTransaction(env.Ctx, func(sess *xorm.Session) error {
		_, err := sess.Exec("DROP TABLE inbox_item")
		return err
	})
	assert.Nil(t, err)

	assert.Nil(t, userDataStore.SyncStructs(new(InboxItem)))
	assert.Nil(t, userDataStore.SyncStructs(new(models.InboxItem)))

	statement := env.NewStatement("csv", []byte("a;b\n"))
	ids := make([]int64, 500)

	for i := range ids {
		ids[i] = 1000000000000000 + int64(i)
	}

	assert.Nil(t, services.InboxItems.MarkInboxItemProcessed(env.Ctx, aitest.Uid, statement.ItemId, ids, ""))

	item, err := services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, statement.ItemId)
	assert.Nil(t, err)
	assert.Equal(t, ids, item.GetResultTransactionIds())
}

func TestInboxItems_SaveProgressKeepsTheItemPending(t *testing.T) {
	env := aitest.NewEnv(t)
	statement := env.NewStatement("csv", []byte("a;b\n"))

	assert.Nil(t, services.InboxItems.SaveInboxItemProgress(env.Ctx, aitest.Uid, statement.ItemId, []int64{11, 22}, "created 2, errors 1 | failed rows: row 2: x"))

	item, err := services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, statement.ItemId)
	assert.Nil(t, err)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PENDING, item.Status)
	assert.Equal(t, []int64{11, 22}, item.GetResultTransactionIds())
	assert.Equal(t, "created 2, errors 1 | failed rows: row 2: x", item.ProcessedNote)
	assert.Equal(t, int64(0), item.ProcessedUnixTime)

	_, statements, err := services.InboxItems.GetPendingCounts(env.Ctx, aitest.Uid)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), statements)

	// only a pending item can be updated
	assert.Nil(t, services.InboxItems.DiscardInboxItem(env.Ctx, aitest.Uid, statement.ItemId, "no"))
	assert.Equal(t, errs.ErrInboxItemNotPending, services.InboxItems.SaveInboxItemProgress(env.Ctx, aitest.Uid, statement.ItemId, []int64{1}, ""))

	assert.Equal(t, models.MergeInboxItemResultTransactionIds([]int64{1, 2}, []int64{2, 3, 3}), []int64{1, 2, 3})
}

func TestInboxItems_DeleteStatementRemovesTheFile(t *testing.T) {
	env := aitest.NewEnv(t)
	statement := env.NewStatement("csv", []byte("a;b\n"))

	assert.Nil(t, services.InboxItems.DeleteInboxItem(env.Ctx, aitest.Uid, statement.ItemId))

	_, _, err := services.InboxItems.ReadInboxItemFile(env.Ctx, statement)
	assert.Equal(t, errs.ErrInboxItemFileNotExists, err)
}

func TestInboxItems_StatementExtensionsAreRestricted(t *testing.T) {
	env := aitest.NewEnv(t)

	item := &models.InboxItem{Uid: aitest.Uid, FileExtension: "exe", OriginalFileName: "x.exe"}
	err := services.InboxItems.CreateStatementItem(env.Ctx, item, aitest.NewMemoryFile([]byte("x")))
	assert.Equal(t, errs.ErrInboxItemFileTypeNotAllowed, err)

	for _, extension := range []string{"pdf", "csv", "xlsx", "xls", "txt", "ofx", "qif"} {
		_, exists := models.InboxStatementFileExtensions[extension]
		assert.True(t, exists, extension)
	}
}

func TestInboxItems_NewReceiptPictureId(t *testing.T) {
	env := aitest.NewEnv(t)
	receipt := env.NewReceipt("")
	statement := env.NewStatement("csv", []byte("a;b\n"))

	pictureId, err := services.InboxItems.GetNewReceiptPictureId(env.Ctx, receipt)
	assert.Nil(t, err)
	assert.Equal(t, receipt.PictureId, pictureId)

	pictureId, err = services.InboxItems.GetNewReceiptPictureId(env.Ctx, statement)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), pictureId)
}
