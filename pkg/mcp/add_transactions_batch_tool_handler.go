package mcp

import (
	"reflect"
	"strings"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport"
	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
)

// MCPAddTransactionsBatchRequest represents all parameters of the add transactions batch request
type MCPAddTransactionsBatchRequest struct {
	Transactions   []*aiimport.TxRow `json:"transactions" jsonschema_description:"Transactions to add (1 to 500)"`
	SourceLabel    string            `json:"source_label" jsonschema_description:"Label of the origin, e.g. bancolombia-2026-10. It is added to the comment of every transaction as [origen: <label>]"`
	SkipDuplicates *bool             `json:"skip_duplicates,omitempty" jsonschema:"default=true" jsonschema_description:"Skip the rows which look like an existing transaction or a previous row of the same request (optional, default true)"`
	InboxItemId    string            `json:"inbox_item_id,omitempty" jsonschema_description:"Id of the pending inbox statement the rows were read from (optional). When dry_run=false and at least one transaction is created, the item becomes processed with the created ids and a summary"`
	DryRun         *bool             `json:"dry_run" jsonschema_description:"If true nothing is saved and every row is only validated. Required"`
}

// MCPAddTransactionsBatchResponse represents the response structure for adding transactions in batch
type MCPAddTransactionsBatchResponse struct {
	DryRun      bool   `json:"dry_run" jsonschema_description:"Indicates whether this operation was a dry run (nothing saved)"`
	SourceLabel string `json:"source_label" jsonschema_description:"Label of the origin"`
	ItemStatus  string `json:"inbox_item_status,omitempty" jsonschema:"enum=pending,enum=processed" jsonschema_description:"Status of the inbox item after the call (only when inbox_item_id was sent)"`
	Warning     string `json:"warning,omitempty" jsonschema_description:"Something that needs attention, e.g. the transactions were saved but the inbox item could not be marked as processed"`
	*aiimport.Result
}

type mcpAddTransactionsBatchToolHandler struct{}

var MCPAddTransactionsBatchToolHandler = &mcpAddTransactionsBatchToolHandler{}

// ToolType returns the MCP tool type
func (h *mcpAddTransactionsBatchToolHandler) ToolType() core.MCPToolType {
	return core.MCP_TOOL_TYPE_ADD_TRANSACTIONS_BATCH
}

// Name returns the name of the MCP tool
func (h *mcpAddTransactionsBatchToolHandler) Name() string {
	return "add_transactions_batch"
}

// Description returns the description of the MCP tool
func (h *mcpAddTransactionsBatchToolHandler) Description() string {
	return "Add many transactions at once (e.g. the rows of a bank statement). Rows that look like an existing transaction (same account, type and amount within 1 day and a similar description) or like a previous row of the request are skipped as duplicates, so importing the same statement twice creates nothing. " +
		"Every created transaction gets the tag 'ia' and a comment ending with [origen: <source_label>]. If the rows come from an inbox statement, send its inbox_item_id: the item becomes processed after the real run. The result has one entry per row with the normalized amount: check it against the document. Never invent accounts, categories, amounts or dates. " + dryRunDescription
}

// InputType returns the input type for the MCP tool request
func (h *mcpAddTransactionsBatchToolHandler) InputType() reflect.Type {
	return reflect.TypeOf(&MCPAddTransactionsBatchRequest{})
}

// OutputType returns the output type for the MCP tool response
func (h *mcpAddTransactionsBatchToolHandler) OutputType() reflect.Type {
	return reflect.TypeOf(&MCPAddTransactionsBatchResponse{})
}

// Handle processes the MCP call tool request and returns the response
func (h *mcpAddTransactionsBatchToolHandler) Handle(c *core.WebContext, callToolReq *MCPCallToolRequest, user *models.User, currentConfig *settings.Config, services MCPAvailableServices) (any, []*MCPTextContent, error) {
	var request MCPAddTransactionsBatchRequest

	if err := unmarshalToolArguments(callToolReq, &request); err != nil {
		return nil, nil, err
	}

	dryRun, err := requireDryRunArgument(request.DryRun)

	if err != nil {
		return nil, nil, err
	}

	sourceLabel := strings.TrimSpace(request.SourceLabel)

	if sourceLabel == "" {
		return nil, nil, errs.NewInboxToolError("source_label is required, e.g. bancolombia-2026-10")
	}

	if err = aiimport.ValidateRowsCount(len(request.Transactions), aiimport.MaxRowsBatch); err != nil {
		return nil, nil, errs.NewInboxToolError("transactions must have between 1 and 500 rows")
	}

	var inboxItemId int64

	if request.InboxItemId != "" {
		inboxItemId, err = parseInboxItemIdArgument(request.InboxItemId)

		if err != nil {
			return nil, nil, err
		}

		// the item must belong to the user (the lookup is by uid) and be a pending statement
		item, err := services.GetInboxItemService().GetInboxItemByItemId(c, user.Uid, inboxItemId)

		if err != nil {
			return nil, nil, err
		}

		if item.Status != models.INBOX_ITEM_STATUS_PENDING {
			return nil, nil, errs.ErrInboxItemNotPending
		}

		if item.Kind != models.INBOX_ITEM_KIND_STATEMENT {
			return nil, nil, errs.NewInboxToolError("inbox_item_id must be a statement, use resolve_inbox_item for receipts")
		}
	}

	skipDuplicates := request.SkipDuplicates == nil || *request.SkipDuplicates

	result, err := aiimport.Import(c, services, user, request.Transactions, &aiimport.Options{
		SourceLabel:    sourceLabel,
		SkipDuplicates: skipDuplicates,
		DryRun:         dryRun,
		ClientIP:       c.ClientIP(),
	})

	if err != nil {
		log.Warnf(c, "[add_transactions_batch_tool_handler.Handle] failed to add transactions for user \"uid:%d\", because %s", user.Uid, err.Error())
		return nil, nil, err
	}

	response := &MCPAddTransactionsBatchResponse{
		DryRun:      dryRun,
		SourceLabel: sourceLabel,
		Result:      result,
	}

	if inboxItemId > 0 {
		response.ItemStatus = inboxStatusPendingName

		if !dryRun && result.Created > 0 {
			response.ItemStatus, response.Warning = aiimport.FinishInboxItem(c, services, user.Uid, inboxItemId, "", result)
		}
	}

	return newJSONTextResponse(response)
}
