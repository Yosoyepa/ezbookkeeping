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

// MCPGetSpendingSummaryRequest represents all parameters of the get spending summary request
type MCPGetSpendingSummaryRequest struct {
	StartTime string `json:"start_time" jsonschema:"format=date-time" jsonschema_description:"Start of the period in RFC 3339 format (e.g. 2026-10-01T00:00:00-05:00)"`
	EndTime   string `json:"end_time" jsonschema:"format=date-time" jsonschema_description:"End of the period in RFC 3339 format (e.g. 2026-10-31T23:59:59-05:00)"`
	Type      string `json:"type" jsonschema:"enum=expense,enum=income" jsonschema_description:"Whether to add up expenses or incomes"`
	GroupBy   string `json:"group_by" jsonschema:"enum=category,enum=month,enum=account,enum=tag,enum=comment_merchant" jsonschema_description:"How to group: category, month (yyyy-mm), account, tag (a transaction with several tags is counted in each of them) or comment_merchant (the description without the origin suffix, lower case and without accents, up to 40 characters)"`
	Currency  string `json:"currency,omitempty" jsonschema_description:"Currency code, only the accounts in this currency are added up (optional, default the currency of the user)"`
	Top       int32  `json:"top,omitempty" jsonschema:"default=20" jsonschema_description:"Maximum number of groups, the rest are added up in the group '(other)' (optional, default 20)"`
}

type mcpGetSpendingSummaryToolHandler struct{}

var MCPGetSpendingSummaryToolHandler = &mcpGetSpendingSummaryToolHandler{}

// ToolType returns the MCP tool type
func (h *mcpGetSpendingSummaryToolHandler) ToolType() core.MCPToolType {
	return core.MCP_TOOL_TYPE_GET_SPENDING_SUMMARY
}

// Name returns the name of the MCP tool
func (h *mcpGetSpendingSummaryToolHandler) Name() string {
	return "get_spending_summary"
}

// Description returns the description of the MCP tool
func (h *mcpGetSpendingSummaryToolHandler) Description() string {
	return "Add up the expenses (or incomes) of a period in the server, grouped by category, month, account, tag or merchant. Use it for insights instead of downloading all the transactions with query_transactions. Transfers are not included."
}

// InputType returns the input type for the MCP tool request
func (h *mcpGetSpendingSummaryToolHandler) InputType() reflect.Type {
	return reflect.TypeOf(&MCPGetSpendingSummaryRequest{})
}

// OutputType returns the output type for the MCP tool response
func (h *mcpGetSpendingSummaryToolHandler) OutputType() reflect.Type {
	return reflect.TypeOf(&aiimport.Summary{})
}

// Handle processes the MCP call tool request and returns the response
func (h *mcpGetSpendingSummaryToolHandler) Handle(c *core.WebContext, callToolReq *MCPCallToolRequest, user *models.User, currentConfig *settings.Config, services MCPAvailableServices) (any, []*MCPTextContent, error) {
	var request MCPGetSpendingSummaryRequest

	if err := unmarshalToolArguments(callToolReq, &request); err != nil {
		return nil, nil, err
	}

	startTime, err := utils.ParseFromLongDateTimeWithTimezoneRFC3339Format(request.StartTime)

	if err != nil {
		return nil, nil, errs.NewInboxToolError("start_time must be in RFC 3339 format (e.g. 2026-10-01T00:00:00-05:00)")
	}

	endTime, err := utils.ParseFromLongDateTimeWithTimezoneRFC3339Format(request.EndTime)

	if err != nil {
		return nil, nil, errs.NewInboxToolError("end_time must be in RFC 3339 format (e.g. 2026-10-31T23:59:59-05:00)")
	}

	if request.Type != aiimport.TransactionTypeExpense && request.Type != aiimport.TransactionTypeIncome {
		return nil, nil, errs.NewInboxToolError("type must be expense or income")
	}

	switch request.GroupBy {
	case aiimport.GroupByCategory, aiimport.GroupByMonth, aiimport.GroupByAccount, aiimport.GroupByTag, aiimport.GroupByCommentMerchant:
	default:
		return nil, nil, errs.NewInboxToolError("group_by must be category, month, account, tag or comment_merchant")
	}

	if endTime.Before(startTime) {
		return nil, nil, errs.NewInboxToolError("end_time must not be before start_time")
	}

	summary, err := aiimport.GetSpendingSummary(c, services, user, &aiimport.SummaryOptions{
		Start:    startTime,
		End:      endTime,
		Type:     request.Type,
		GroupBy:  request.GroupBy,
		Currency: request.Currency,
		Top:      int(request.Top),
	})

	if err != nil {
		log.Errorf(c, "[get_spending_summary_tool_handler.Handle] failed to get spending summary for user \"uid:%d\", because %s", user.Uid, err.Error())
		return nil, nil, err
	}

	return newJSONTextResponse(summary)
}
