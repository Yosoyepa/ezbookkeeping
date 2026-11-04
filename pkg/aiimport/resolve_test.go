package aiimport

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport/aitest"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/services"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

func getInboxItem(t *testing.T, env *aitest.Env, itemId int64) *models.InboxItem {
	t.Helper()
	item, err := services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, itemId)
	assert.Nil(t, err)

	return item
}

func TestResolveInboxItem_DryRunWritesNothing(t *testing.T) {
	env := aitest.NewEnv(t)
	item := env.NewReceipt("recibo del Éxito")

	result, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, []*TxRow{newExpenseRow("45.900", "Éxito", "2026-10-15T12:00:00-05:00")}, "", true, true, "127.0.0.1")
	assert.Nil(t, err)
	assert.Equal(t, 1, result.WouldCreate)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, "pending", result.ItemStatus)
	assert.Equal(t, utils.Int64ToString(item.ItemId), result.InboxItemId)

	assert.Equal(t, int64(0), env.CountTransactions())
	assert.Equal(t, models.INBOX_ITEM_STATUS_PENDING, getInboxItem(t, env, item.ItemId).Status)
	assert.Equal(t, 0, countTagsNamed(t, env, AITagName))

	newPictures, err := services.TransactionPictures.GetNewPictureInfosByPictureIds(env.Ctx, aitest.Uid, []int64{item.PictureId})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(newPictures)) // the picture is still not linked to any transaction
}

func TestResolveInboxItem_RealProcessesTheItemAndLinksThePicture(t *testing.T) {
	env := aitest.NewEnv(t)
	item := env.NewReceipt("")

	rows := []*TxRow{
		newExpenseRow("45.900", "Éxito Suba", "2026-10-15T12:00:00-05:00"),
		newExpenseRow("12.000", "Tinto", "2026-10-15T12:30:00-05:00"),
	}

	result, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, rows, "dos compras", true, false, "127.0.0.1")
	assert.Nil(t, err)
	assert.Equal(t, 2, result.Created)
	assert.Equal(t, "processed", result.ItemStatus)
	assert.Empty(t, result.Warning)

	processed := getInboxItem(t, env, item.ItemId)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PROCESSED, processed.Status)
	assert.Equal(t, []int64{result.CreatedTransactionIds[0], result.CreatedTransactionIds[1]}, processed.GetResultTransactionIds())
	assert.Contains(t, processed.ProcessedNote, "dos compras")
	assert.Contains(t, processed.ProcessedNote, "created 2")
	assert.Greater(t, processed.ProcessedUnixTime, int64(0))

	first := getTransaction(t, env, result.Rows[0].TransactionId)
	assert.Equal(t, "Éxito Suba [origen: inbox:"+utils.Int64ToString(item.ItemId)+"]", first.Comment)
	assert.Equal(t, []string{AITagName}, getTagNames(t, env, result.Rows[0].TransactionId))

	// the picture is linked to the first transaction only
	pictures, err := services.TransactionPictures.GetPictureInfosByTransactionId(env.Ctx, aitest.Uid, first.TransactionId)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(pictures))
	assert.Equal(t, item.PictureId, pictures[0].PictureId)

	secondId, _ := utils.StringToInt64(result.Rows[1].TransactionId)
	pictures, err = services.TransactionPictures.GetPictureInfosByTransactionId(env.Ctx, aitest.Uid, secondId)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(pictures))

	// the receipt cannot be resolved twice
	_, err = ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, rows, "", true, false, "127.0.0.1")
	assert.Equal(t, errs.ErrInboxItemNotPending, err)
	assert.Equal(t, int64(2), env.CountTransactions())
}

func TestResolveInboxItem_AllDuplicatesKeepsTheItemPending(t *testing.T) {
	env := aitest.NewEnv(t)
	_, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("45.900", "Éxito Suba", "2026-10-15T12:00:00-05:00")}, importOptions("manual", false, false))
	assert.Nil(t, err)

	item := env.NewReceipt("")
	rows := []*TxRow{newExpenseRow("45.900", "Éxito", "2026-10-15T12:00:00-05:00")}

	result, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, rows, "", true, false, "127.0.0.1")
	assert.Nil(t, err)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.Duplicates)
	assert.Equal(t, "pending", result.ItemStatus)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PENDING, getInboxItem(t, env, item.ItemId).Status)

	// the user can force the creation
	result, err = ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, rows, "", false, false, "127.0.0.1")
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, "processed", result.ItemStatus)
}

// H4: if any row fails the item stays pending, with the created ids and the failed rows saved
func TestResolveInboxItem_PartialErrorsKeepTheItemPending(t *testing.T) {
	env := aitest.NewEnv(t)
	item := env.NewReceipt("")

	rows := []*TxRow{
		newExpenseRow("45.900", "Éxito", "2026-10-15T12:00:00-05:00"),
		newExpenseRow("1.2.3", "malo", "2026-10-15T12:00:00-05:00"),
	}

	result, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, rows, "dos compras", true, false, "127.0.0.1")
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, result.Errors)
	assert.Equal(t, "pending", result.ItemStatus)
	assert.Contains(t, result.NextStep, "fix the failed rows and call again")
	assert.Empty(t, result.Warning)

	pending := getInboxItem(t, env, item.ItemId)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PENDING, pending.Status)
	assert.Equal(t, result.CreatedTransactionIds, pending.GetResultTransactionIds())
	assert.Contains(t, pending.ProcessedNote, "dos compras")
	assert.Contains(t, pending.ProcessedNote, "errors 1")
	assert.Contains(t, pending.ProcessedNote, "row 1:")
	assert.Equal(t, int64(0), pending.ProcessedUnixTime)

	// the failed rows are still in the pending inbox
	receipts, _, err := services.InboxItems.GetPendingCounts(env.Ctx, aitest.Uid)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), receipts)

	// the call can be repeated: the created row is a duplicate, the fixed row is created and the item is processed with all the ids
	rows[1] = newExpenseRow("12.000", "Tinto", "2026-10-15T12:30:00-05:00")
	retry, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, rows, "", true, false, "127.0.0.1")
	assert.Nil(t, err)
	assert.Equal(t, 1, retry.Created)
	assert.Equal(t, 1, retry.Duplicates)
	assert.Equal(t, 0, retry.Errors)
	assert.Equal(t, "processed", retry.ItemStatus)
	assert.Empty(t, retry.NextStep)
	assert.Equal(t, int64(2), env.CountTransactions())

	processed := getInboxItem(t, env, item.ItemId)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PROCESSED, processed.Status)
	assert.Equal(t, []int64{result.CreatedTransactionIds[0], retry.CreatedTransactionIds[0]}, processed.GetResultTransactionIds())
	assert.Greater(t, processed.ProcessedUnixTime, int64(0))
	assert.NotContains(t, processed.ProcessedNote, "row 1:")
}

func TestResolveInboxItem_OnlyFailedRowsKeepTheItemPending(t *testing.T) {
	env := aitest.NewEnv(t)
	item := env.NewStatement("csv", []byte("fecha;valor\n"))

	result, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, []*TxRow{newExpenseRow("1.2.3", "malo", "2026-10-15T12:00:00-05:00")}, "", true, false, "127.0.0.1")
	assert.Nil(t, err)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.Errors)
	assert.Equal(t, "pending", result.ItemStatus)
	assert.NotEmpty(t, result.NextStep)

	pending := getInboxItem(t, env, item.ItemId)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PENDING, pending.Status)
	assert.Empty(t, pending.GetResultTransactionIds())
	assert.Contains(t, pending.ProcessedNote, "row 0:")
}

// H4: the rows which are duplicates are not errors, so they do not keep the item pending
func TestResolveInboxItem_DuplicatesDoNotKeepTheItemPending(t *testing.T) {
	env := aitest.NewEnv(t)
	_, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("45.900", "Éxito Suba", "2026-10-15T12:00:00-05:00")}, importOptions("manual", false, false))
	assert.Nil(t, err)

	item := env.NewStatement("csv", []byte("fecha;valor\n"))
	rows := []*TxRow{
		newExpenseRow("45.900", "Éxito", "2026-10-15T12:00:00-05:00"), // duplicate
		newExpenseRow("12.000", "Tinto", "2026-10-15T12:30:00-05:00"),
	}

	result, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, rows, "", true, false, "127.0.0.1")
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, result.Duplicates)
	assert.Equal(t, "processed", result.ItemStatus)
	assert.Empty(t, result.NextStep)
}

func TestImportStatementRows_PartialErrorsKeepTheItemPending(t *testing.T) {
	env := aitest.NewEnv(t)
	item := env.NewStatement("csv", []byte("fecha;valor\n"))
	receipt := env.NewReceipt("")

	rows := []*TxRow{
		newExpenseRow("45.900", "Éxito", "2026-10-15T12:00:00-05:00"),
		newExpenseRow("12.000", "Videojuegos", "2026-10-15T13:00:00-05:00"),
	}
	rows[1].SecondaryCategoryName = "Videojuegos" // the category does not exist

	result, err := ImportStatementRows(env.Ctx, aitest.Services{}, env.User, item.ItemId, rows, importOptions("bancolombia-2026-10", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, result.Errors)
	assert.Equal(t, "pending", result.ItemStatus)
	assert.Contains(t, result.NextStep, "fix the failed rows and call again")

	pending := getInboxItem(t, env, item.ItemId)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PENDING, pending.Status)
	assert.Equal(t, 1, len(pending.GetResultTransactionIds()))
	assert.Contains(t, pending.ProcessedNote, "row 1:")

	// fixing the row and sending everything again completes the item
	rows[1].SecondaryCategoryName = "Taxi"
	retry, err := ImportStatementRows(env.Ctx, aitest.Services{}, env.User, item.ItemId, rows, importOptions("bancolombia-2026-10", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, retry.Created)
	assert.Equal(t, 1, retry.Duplicates)
	assert.Equal(t, "processed", retry.ItemStatus)
	assert.Equal(t, 2, len(getInboxItem(t, env, item.ItemId).GetResultTransactionIds()))
	assert.Equal(t, int64(2), env.CountTransactions())

	// a receipt is not accepted by the statement path, and a processed statement cannot be used again
	_, err = ImportStatementRows(env.Ctx, aitest.Services{}, env.User, receipt.ItemId, rows, importOptions("x", true, false))
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "must be a statement")

	_, err = ImportStatementRows(env.Ctx, aitest.Services{}, env.User, item.ItemId, rows, importOptions("x", true, false))
	assert.Equal(t, errs.ErrInboxItemNotPending, err)
}

// H2: two simultaneous calls on the same item create the transactions once, the other one fails cleanly
func TestResolveInboxItem_ConcurrentCallsOnTheSameItemCreateOnce(t *testing.T) {
	env := aitest.NewEnv(t)
	item := env.NewStatement("csv", []byte("fecha;valor\n"))

	newRows := func() []*TxRow {
		return []*TxRow{
			newExpenseRow("45.900", "Éxito Suba", "2026-10-15T12:00:00-05:00"),
			newExpenseRow("12.500", "Tinto Juan Valdez", "2026-10-16T08:30:00-05:00"),
		}
	}

	const callers = 2

	results := make([]*ResolveResult, callers)
	callErrors := make([]error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < callers; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()
			<-start
			results[i], callErrors[i] = ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, newRows(), "", true, false, "127.0.0.1")
		}(i)
	}

	close(start)
	wg.Wait()

	succeeded, failed := 0, 0

	for i := 0; i < callers; i++ {
		if callErrors[i] == nil {
			succeeded++
			assert.Equal(t, 2, results[i].Created)
			assert.Equal(t, "processed", results[i].ItemStatus)
		} else {
			failed++
			assert.Equal(t, errs.ErrInboxItemNotPending, callErrors[i])
		}
	}

	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 1, failed)
	assert.Equal(t, int64(2), env.CountTransactions())
	assert.Equal(t, 2, len(getInboxItem(t, env, item.ItemId).GetResultTransactionIds()))
}

// H2: the item is reserved before writing, a change made after the item was read makes the call fail before creating anything
func TestReserveInboxItem_FailsWhenTheItemChanged(t *testing.T) {
	env := aitest.NewEnv(t)
	item := env.NewStatement("csv", []byte("fecha;valor\n"))
	read := getInboxItem(t, env, item.ItemId)

	assert.Nil(t, services.InboxItems.ReserveInboxItem(env.Ctx, aitest.Uid, item.ItemId, read.UpdatedUnixTime))

	// the same (now stale) version cannot reserve it again, even in the same second
	assert.Equal(t, errs.ErrInboxItemNotPending, services.InboxItems.ReserveInboxItem(env.Ctx, aitest.Uid, item.ItemId, read.UpdatedUnixTime))
	assert.Greater(t, getInboxItem(t, env, item.ItemId).UpdatedUnixTime, read.UpdatedUnixTime)

	// an item discarded after being read cannot be reserved
	current := getInboxItem(t, env, item.ItemId)
	assert.Nil(t, services.InboxItems.DiscardInboxItem(env.Ctx, aitest.Uid, item.ItemId, "no sirve"))
	assert.Equal(t, errs.ErrInboxItemNotPending, services.InboxItems.ReserveInboxItem(env.Ctx, aitest.Uid, item.ItemId, current.UpdatedUnixTime))
}

func TestResolveInboxItem_RowLimits(t *testing.T) {
	env := aitest.NewEnv(t)
	receipt := env.NewReceipt("")
	statement := env.NewStatement("csv", []byte("fecha;valor\n"))

	buildRows := func(count int) []*TxRow {
		rows := make([]*TxRow, count)

		for i := range rows {
			rows[i] = newExpenseRow("1.000", "x", "2026-10-15T12:00:00-05:00")
		}

		return rows
	}

	_, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, receipt.ItemId, nil, "", true, true, "")
	assert.Equal(t, errs.ErrIncompleteOrIncorrectSubmission, err)

	_, err = ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, receipt.ItemId, buildRows(MaxRowsResolveReceipt+1), "", true, true, "")
	assert.Equal(t, errs.ErrIncompleteOrIncorrectSubmission, err)

	result, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, statement.ItemId, buildRows(MaxRowsResolveReceipt+1), "", false, true, "")
	assert.Nil(t, err)
	assert.Equal(t, MaxRowsResolveReceipt+1, result.WouldCreate)

	_, err = ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, statement.ItemId, buildRows(MaxRowsResolveStatement+1), "", false, true, "")
	assert.Equal(t, errs.ErrIncompleteOrIncorrectSubmission, err)
}

func TestResolveInboxItem_UnknownItem(t *testing.T) {
	env := aitest.NewEnv(t)

	_, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, 123456, []*TxRow{newExpenseRow("1.000", "x", "2026-10-15T12:00:00-05:00")}, "", true, true, "")
	assert.Equal(t, errs.ErrInboxItemNotFound, err)
}

func TestResolveInboxItem_ReopenedReceiptCanBeResolvedAgain(t *testing.T) {
	env := aitest.NewEnv(t)
	item := env.NewReceipt("")

	_, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, []*TxRow{newExpenseRow("45.900", "Éxito", "2026-10-15T12:00:00-05:00")}, "", true, false, "")
	assert.Nil(t, err)

	assert.Nil(t, services.InboxItems.ReopenInboxItem(env.Ctx, aitest.Uid, item.ItemId))
	assert.Equal(t, models.INBOX_ITEM_STATUS_PENDING, getInboxItem(t, env, item.ItemId).Status)
	assert.Equal(t, int64(1), env.CountTransactions()) // reopening does not delete the transaction

	// the picture is already linked to the first transaction, so a second resolve creates the new transaction without a picture
	result, err := ResolveInboxItem(env.Ctx, aitest.Services{}, env.User, item.ItemId, []*TxRow{newExpenseRow("12.000", "Tinto", "2026-10-16T12:00:00-05:00")}, "", true, false, "")
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, "processed", result.ItemStatus)
}
