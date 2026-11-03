package services_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport/aitest"
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
