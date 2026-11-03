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

// ResolveResult represents the result of resolving an inbox item
type ResolveResult struct {
	*Result
	InboxItemId string `json:"inbox_item_id" jsonschema_description:"Id of the inbox item"`
	ItemStatus  string `json:"inbox_item_status" jsonschema:"enum=pending,enum=processed" jsonschema_description:"Status of the inbox item after the call. It only becomes processed when at least one transaction was created"`
	Warning     string `json:"warning,omitempty" jsonschema_description:"Something that needs attention, e.g. the transactions were saved but the item could not be marked as processed"`
}

// ResolveInboxItem creates the transactions of the rows from the pending inbox item. When the item is a receipt its picture is linked to the first transaction created
func ResolveInboxItem(c core.Context, svc Services, user *models.User, itemId int64, rows []*TxRow, note string, skipDuplicates bool, dryRun bool, clientIP string) (*ResolveResult, error) {
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

	result, err := Import(c, svc, user, rows, &Options{
		SourceLabel:    fmt.Sprintf("inbox:%d", itemId),
		SkipDuplicates: skipDuplicates,
		DryRun:         dryRun,
		ClientIP:       clientIP,
		PictureId:      pictureId,
	})

	if err != nil {
		return nil, err
	}

	resolveResult := &ResolveResult{
		Result:      result,
		InboxItemId: utils.Int64ToString(itemId),
		ItemStatus:  "pending",
	}

	if dryRun || result.Created < 1 {
		return resolveResult, nil
	}

	resolveResult.ItemStatus, resolveResult.Warning = FinishInboxItem(c, svc, user.Uid, itemId, note, result)

	return resolveResult, nil
}

// FinishInboxItem marks the pending inbox item as processed with the ids of the created transactions and a summary in its processed note.
// It returns the status of the item ("processed", or "pending" if it could not be saved) and a warning in that case
func FinishInboxItem(c core.Context, svc Services, uid int64, itemId int64, note string, result *Result) (string, string) {
	err := svc.GetInboxItemService().MarkInboxItemProcessed(c, uid, itemId, result.CreatedTransactionIds, buildProcessedNote(note, result))

	if err != nil {
		log.Errorf(c, "[aiimport.FinishInboxItem] failed to mark inbox item \"id:%d\" as processed for user \"uid:%d\", because %s", itemId, uid, err.Error())
		return "pending", "the transactions were created but the inbox item could not be marked as processed: " + err.Error()
	}

	return "processed", ""
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
		parts = append(parts, FormatRowErrors(result, 3))
	}

	return strings.Join(parts, " | ")
}
