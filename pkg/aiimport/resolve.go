package aiimport

import (
	"fmt"
	"strings"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// Maximum count of rows when resolving an inbox item
const (
	MaxRowsResolveReceipt   = 50
	MaxRowsResolveStatement = 500
	MaxRowsBatch            = 500
)

// Status names of an inbox item in the results
const (
	ItemStatusPending   = "pending"
	ItemStatusProcessed = "processed"
)

// maxFailedRowsInNote is the max count of failed rows listed in the processed note of an inbox item
const maxFailedRowsInNote = 10

// nextStepFixFailedRows is the instruction for the CLI when some rows failed and the inbox item stays pending
const nextStepFixFailedRows = "fix the failed rows and call again with the same item: the rows already created are skipped as duplicates, and the item becomes processed when no row fails"

// ItemOutcome represents what happened to the inbox item after importing the rows of it
type ItemOutcome struct {
	ItemStatus string `json:"inbox_item_status,omitempty" jsonschema:"enum=pending,enum=processed" jsonschema_description:"Status of the inbox item after the call. It becomes processed when no row failed and at least one transaction was created (now or in a previous call); if any row failed it stays pending"`
	Warning    string `json:"warning,omitempty" jsonschema_description:"Something that needs attention, e.g. the transactions were saved but the item could not be updated"`
	NextStep   string `json:"next_step,omitempty" jsonschema_description:"What to do next when the inbox item stays pending because some rows failed"`
}

// ResolveResult represents the result of resolving an inbox item
type ResolveResult struct {
	*Result
	InboxItemId string `json:"inbox_item_id" jsonschema_description:"Id of the inbox item"`
	ItemOutcome
}

// StatementResult represents the result of importing the rows of an inbox statement
type StatementResult struct {
	*Result
	ItemOutcome
}

// ResolveInboxItem creates the transactions of the rows from the pending inbox item. When the item is a receipt its picture is linked to the first transaction created.
// If any row fails the item stays pending (with the created ids and the failed rows saved), so the rows can be fixed and the call repeated
func ResolveInboxItem(c core.Context, svc Services, user *models.User, itemId int64, rows []*TxRow, note string, skipDuplicates bool, dryRun bool, clientIP string) (*ResolveResult, error) {
	defer lockUser(user.Uid)()

	item, err := svc.GetInboxItemService().GetInboxItemByItemId(c, user.Uid, itemId)

	if err != nil {
		return nil, err
	}

	if item.Status != models.INBOX_ITEM_STATUS_PENDING {
		return nil, errs.ErrInboxItemNotPending
	}

	maxRows := MaxRowsResolveReceipt

	if item.Kind == models.INBOX_ITEM_KIND_STATEMENT {
		maxRows = MaxRowsResolveStatement
	}

	if err = ValidateRowsCount(len(rows), maxRows); err != nil {
		return nil, err
	}

	pictureId, err := svc.GetInboxItemService().GetNewReceiptPictureId(c, item)

	if err != nil {
		return nil, err
	}

	result, outcome, err := importForItem(c, svc, user, item, rows, note, &Options{
		SourceLabel:    fmt.Sprintf("inbox:%d", itemId),
		SkipDuplicates: skipDuplicates,
		DryRun:         dryRun,
		ClientIP:       clientIP,
		PictureId:      pictureId,
	})

	if err != nil {
		return nil, err
	}

	return &ResolveResult{
		Result:      result,
		InboxItemId: utils.Int64ToString(itemId),
		ItemOutcome: outcome,
	}, nil
}

// ImportStatementRows imports the rows read from a pending inbox statement (add_transactions_batch with inbox_item_id).
// The check of the item, the check of the duplicates, the writes and the update of the item run together for the user
func ImportStatementRows(c core.Context, svc Services, user *models.User, itemId int64, rows []*TxRow, options *Options) (*StatementResult, error) {
	defer lockUser(user.Uid)()

	// the item must belong to the user (the lookup is by uid) and be a pending statement
	item, err := svc.GetInboxItemService().GetInboxItemByItemId(c, user.Uid, itemId)

	if err != nil {
		return nil, err
	}

	if item.Status != models.INBOX_ITEM_STATUS_PENDING {
		return nil, errs.ErrInboxItemNotPending
	}

	if item.Kind != models.INBOX_ITEM_KIND_STATEMENT {
		return nil, errs.NewInboxToolError("inbox_item_id must be a statement, use resolve_inbox_item for receipts")
	}

	result, outcome, err := importForItem(c, svc, user, item, rows, "", options)

	if err != nil {
		return nil, err
	}

	return &StatementResult{
		Result:      result,
		ItemOutcome: outcome,
	}, nil
}

// FinishInboxItem updates the pending inbox item with the result of importing its rows: it becomes processed when no row failed (see finishInboxItem).
// It returns the status of the item, a warning and the next step
func FinishInboxItem(c core.Context, svc Services, uid int64, itemId int64, note string, result *Result) ItemOutcome {
	defer lockUser(uid)()

	item, err := svc.GetInboxItemService().GetInboxItemByItemId(c, uid, itemId)

	if err != nil {
		log.Errorf(c, "[aiimport.FinishInboxItem] failed to get inbox item \"id:%d\" for user \"uid:%d\", because %s", itemId, uid, err.Error())
		return ItemOutcome{ItemStatus: ItemStatusPending, Warning: "the transactions were created but the inbox item could not be updated: " + err.Error()}
	}

	return finishInboxItem(c, svc, item, note, result)
}

// importForItem imports the rows of the pending inbox item, the caller holds the lock of the user.
// Before writing anything the item is reserved with a conditional update, so a change of the item made after it was read makes the call fail before creating transactions
func importForItem(c core.Context, svc Services, user *models.User, item *models.InboxItem, rows []*TxRow, note string, options *Options) (*Result, ItemOutcome, error) {
	if !options.DryRun {
		if err := svc.GetInboxItemService().ReserveInboxItem(c, user.Uid, item.ItemId, item.UpdatedUnixTime); err != nil {
			return nil, ItemOutcome{}, err
		}
	}

	result, err := importLocked(c, svc, user, rows, options)

	if err != nil {
		return nil, ItemOutcome{}, err
	}

	if options.DryRun {
		return result, ItemOutcome{ItemStatus: ItemStatusPending}, nil
	}

	return result, finishInboxItem(c, svc, item, note, result), nil
}

// finishInboxItem saves the result of importing the rows in the inbox item, the caller holds the lock of the user.
//   - If any row failed, the item stays pending: the ids created so far (added to the previous ones) and the failed rows are saved, and the next step tells to fix the rows and call again.
//     The rows already created are skipped as duplicates in the next call, so repeating it is safe.
//   - If no row failed and there are transactions created (now or in a previous call), the item becomes processed with all the ids.
//   - Otherwise (everything was a duplicate) nothing changes.
func finishInboxItem(c core.Context, svc Services, item *models.InboxItem, note string, result *Result) ItemOutcome {
	uid := item.Uid
	itemId := item.ItemId
	allIds := models.MergeInboxItemResultTransactionIds(item.GetResultTransactionIds(), result.CreatedTransactionIds)
	outcome := ItemOutcome{ItemStatus: ItemStatusPending}

	if result.Errors > 0 {
		outcome.NextStep = nextStepFixFailedRows

		err := svc.GetInboxItemService().SaveInboxItemProgress(c, uid, itemId, allIds, buildProcessedNote(note, result))

		if err != nil {
			log.Errorf(c, "[aiimport.finishInboxItem] failed to save the progress of inbox item \"id:%d\" for user \"uid:%d\", because %s", itemId, uid, err.Error())
			outcome.Warning = "the item could not be updated with the failed rows: " + err.Error()
		}

		return outcome
	}

	if len(allIds) < 1 {
		return outcome
	}

	err := svc.GetInboxItemService().MarkInboxItemProcessed(c, uid, itemId, allIds, buildProcessedNote(note, result))

	if err != nil {
		log.Errorf(c, "[aiimport.finishInboxItem] failed to mark inbox item \"id:%d\" as processed for user \"uid:%d\", because %s", itemId, uid, err.Error())
		outcome.Warning = "the transactions were created but the inbox item could not be marked as processed: " + err.Error()

		return outcome
	}

	outcome.ItemStatus = ItemStatusProcessed

	return outcome
}

func buildProcessedNote(note string, result *Result) string {
	parts := make([]string, 0, 3)

	if strings.TrimSpace(note) != "" {
		parts = append(parts, strings.TrimSpace(note))
	}

	summary := fmt.Sprintf("created %d", result.Created)

	if result.Duplicates > 0 {
		summary += fmt.Sprintf(", duplicates %d", result.Duplicates)
	}

	if result.Errors > 0 {
		summary += fmt.Sprintf(", errors %d", result.Errors)
	}

	parts = append(parts, summary)

	if result.Errors > 0 {
		parts = append(parts, "failed rows: "+FormatRowErrors(result, maxFailedRowsInNote))
	}

	return strings.Join(parts, " | ")
}
