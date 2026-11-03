package mcp

import (
	"reflect"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport"
	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// MCPUpdateTransactionRequest represents all parameters of the update transaction request
type MCPUpdateTransactionRequest struct {
	Id           string    `json:"id" jsonschema_description:"Transaction id (from query_transactions, add_transactions_batch or resolve_inbox_item)"`
	CategoryName *string   `json:"category_name,omitempty" jsonschema_description:"New secondary category name (optional)"`
	Tags         *[]string `json:"tags,omitempty" jsonschema_description:"New list of tag names, it REPLACES the current tags, an empty list removes them (optional)"`
	Comment      *string   `json:"comment,omitempty" jsonschema_description:"New description (optional). The [origen: ...] suffix of the transaction is kept"`
	Amount       *string   `json:"amount,omitempty" jsonschema_description:"New amount (optional, Colombian format accepted)"`
	Time         *string   `json:"time,omitempty" jsonschema:"format=date-time" jsonschema_description:"New time in RFC 3339 format (optional)"`
	AccountName  *string   `json:"account_name,omitempty" jsonschema_description:"New account name, it must use the same currency (optional)"`
	DryRun       *bool     `json:"dry_run" jsonschema_description:"If true nothing is saved and the transaction is only validated. Required"`
}

// MCPUpdateTransactionResponse represents the response structure for updating a transaction
type MCPUpdateTransactionResponse struct {
	Success bool                      `json:"success" jsonschema_description:"Indicates whether this operation is successful"`
	DryRun  bool                      `json:"dry_run" jsonschema_description:"Indicates whether this operation was a dry run (nothing saved)"`
	Changed bool                      `json:"changed" jsonschema_description:"Whether the transaction is different after the update"`
	Before  *aiimport.TransactionInfo `json:"before" jsonschema_description:"The transaction before the update"`
	After   *aiimport.TransactionInfo `json:"after" jsonschema_description:"The transaction after the update"`
}

type mcpUpdateTransactionToolHandler struct{}

var MCPUpdateTransactionToolHandler = &mcpUpdateTransactionToolHandler{}

// ToolType returns the MCP tool type
func (h *mcpUpdateTransactionToolHandler) ToolType() core.MCPToolType {
	return core.MCP_TOOL_TYPE_UPDATE_TRANSACTION
}

// Name returns the name of the MCP tool
func (h *mcpUpdateTransactionToolHandler) Name() string {
	return "update_transaction"
}

// Description returns the description of the MCP tool
func (h *mcpUpdateTransactionToolHandler) Description() string {
	return "Change the category, tags, comment, amount, time or account of an existing income, expense or transfer transaction (e.g. to recategorize). Only the fields sent are changed. The response shows the transaction before and after. " + dryRunDescription
}

// InputType returns the input type for the MCP tool request
func (h *mcpUpdateTransactionToolHandler) InputType() reflect.Type {
	return reflect.TypeOf(&MCPUpdateTransactionRequest{})
}

// OutputType returns the output type for the MCP tool response
func (h *mcpUpdateTransactionToolHandler) OutputType() reflect.Type {
	return reflect.TypeOf(&MCPUpdateTransactionResponse{})
}

// Handle processes the MCP call tool request and returns the response
func (h *mcpUpdateTransactionToolHandler) Handle(c *core.WebContext, callToolReq *MCPCallToolRequest, user *models.User, currentConfig *settings.Config, services MCPAvailableServices) (any, []*MCPTextContent, error) {
	var request MCPUpdateTransactionRequest

	if err := unmarshalToolArguments(callToolReq, &request); err != nil {
		return nil, nil, err
	}

	transactionId, err := utils.StringToInt64(request.Id)

	if err != nil || transactionId <= 0 {
		return nil, nil, errs.ErrTransactionIdInvalid
	}

	dryRun, err := requireDryRunArgument(request.DryRun)

	if err != nil {
		return nil, nil, err
	}

	result, err := aiimport.UpdateTransaction(c, services, user, &aiimport.UpdateOptions{
		TransactionId: transactionId,
		CategoryName:  request.CategoryName,
		Tags:          request.Tags,
		Comment:       request.Comment,
		Amount:        request.Amount,
		Time:          request.Time,
		AccountName:   request.AccountName,
		DryRun:        dryRun,
	})

	if err != nil {
		log.Warnf(c, "[update_transaction_tool_handler.Handle] failed to update transaction \"id:%d\" for user \"uid:%d\", because %s", transactionId, user.Uid, err.Error())
		return nil, nil, err
	}

	return newJSONTextResponse(&MCPUpdateTransactionResponse{
		Success: true,
		DryRun:  dryRun,
		Changed: result.Changed,
		Before:  result.Before,
		After:   result.After,
	})
}
