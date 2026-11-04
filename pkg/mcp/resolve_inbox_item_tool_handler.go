package mcp

import (
	"reflect"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport"
	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
)

// MCPResolveInboxItemRequest represents all parameters of the resolve inbox item request
type MCPResolveInboxItemRequest struct {
	Id             string            `json:"id" jsonschema_description:"Inbox item id"`
	Transactions   []*aiimport.TxRow `json:"transactions" jsonschema_description:"Transactions read from the receipt or statement (1 to 50 for a receipt, up to 500 for a statement)"`
	Note           string            `json:"note,omitempty" jsonschema_description:"Note saved in the inbox item (optional)"`
	SkipDuplicates *bool             `json:"skip_duplicates,omitempty" jsonschema:"default=true" jsonschema_description:"Skip the rows which look like an existing transaction (optional, default true)"`
	DryRun         *bool             `json:"dry_run" jsonschema_description:"If true nothing is saved and every row is only validated. Required"`
}

type mcpResolveInboxItemToolHandler struct{}

var MCPResolveInboxItemToolHandler = &mcpResolveInboxItemToolHandler{}

// ToolType returns the MCP tool type
func (h *mcpResolveInboxItemToolHandler) ToolType() core.MCPToolType {
	return core.MCP_TOOL_TYPE_RESOLVE_INBOX_ITEM
}

// Name returns the name of the MCP tool
func (h *mcpResolveInboxItemToolHandler) Name() string {
	return "resolve_inbox_item"
}

// Description returns the description of the MCP tool
func (h *mcpResolveInboxItemToolHandler) Description() string {
	return "Create the transactions read from a pending inbox item (receipt or statement). When at least one transaction is created the item becomes processed and the receipt photo is attached to the first transaction. " +
		"Every created transaction gets the tag 'ia' and a comment ending with [origen: inbox:<id>]. Never invent accounts, categories, amounts or dates. " + dryRunDescription
}

// InputType returns the input type for the MCP tool request
func (h *mcpResolveInboxItemToolHandler) InputType() reflect.Type {
	return reflect.TypeOf(&MCPResolveInboxItemRequest{})
}

// OutputType returns the output type for the MCP tool response
func (h *mcpResolveInboxItemToolHandler) OutputType() reflect.Type {
	return reflect.TypeOf(&aiimport.ResolveResult{})
}

// Handle processes the MCP call tool request and returns the response
func (h *mcpResolveInboxItemToolHandler) Handle(c *core.WebContext, callToolReq *MCPCallToolRequest, user *models.User, currentConfig *settings.Config, services MCPAvailableServices) (any, []*MCPTextContent, error) {
	var request MCPResolveInboxItemRequest

	if err := unmarshalToolArguments(callToolReq, &request); err != nil {
		return nil, nil, err
	}

	itemId, err := parseInboxItemIdArgument(request.Id)

	if err != nil {
		return nil, nil, err
	}

	dryRun, err := requireDryRunArgument(request.DryRun)

	if err != nil {
		return nil, nil, err
	}

	skipDuplicates := request.SkipDuplicates == nil || *request.SkipDuplicates

	result, err := aiimport.ResolveInboxItem(c, services, user, itemId, request.Transactions, request.Note, skipDuplicates, dryRun, c.ClientIP())

	if err != nil {
		log.Warnf(c, "[resolve_inbox_item_tool_handler.Handle] failed to resolve inbox item \"id:%d\" for user \"uid:%d\", because %s", itemId, user.Uid, err.Error())
		return nil, nil, err
	}

	return newJSONTextResponse(result)
}
