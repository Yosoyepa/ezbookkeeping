package mcp

import (
	"reflect"
	"strings"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// MCPDiscardInboxItemRequest represents all parameters of the discard inbox item request
type MCPDiscardInboxItemRequest struct {
	Id     string `json:"id" jsonschema_description:"Inbox item id"`
	Reason string `json:"reason" jsonschema_description:"Why the item is discarded (e.g. not a receipt, unreadable, already registered)"`
}

// MCPDiscardInboxItemResponse represents the response structure for discarding an inbox item
type MCPDiscardInboxItemResponse struct {
	Success bool   `json:"success" jsonschema_description:"Indicates whether this operation is successful"`
	Id      string `json:"id" jsonschema_description:"Inbox item id"`
	Status  string `json:"status" jsonschema:"enum=discarded" jsonschema_description:"Status of the item after the call"`
}

type mcpDiscardInboxItemToolHandler struct{}

var MCPDiscardInboxItemToolHandler = &mcpDiscardInboxItemToolHandler{}

// ToolType returns the MCP tool type
func (h *mcpDiscardInboxItemToolHandler) ToolType() core.MCPToolType {
	return core.MCP_TOOL_TYPE_DISCARD_INBOX_ITEM
}

// Name returns the name of the MCP tool
func (h *mcpDiscardInboxItemToolHandler) Name() string {
	return "discard_inbox_item"
}

// Description returns the description of the MCP tool
func (h *mcpDiscardInboxItemToolHandler) Description() string {
	return "Discard a pending inbox item that must not create transactions (not a receipt, unreadable, already registered). The user can reopen it later from the app. Ask the user before discarding."
}

// InputType returns the input type for the MCP tool request
func (h *mcpDiscardInboxItemToolHandler) InputType() reflect.Type {
	return reflect.TypeOf(&MCPDiscardInboxItemRequest{})
}

// OutputType returns the output type for the MCP tool response
func (h *mcpDiscardInboxItemToolHandler) OutputType() reflect.Type {
	return reflect.TypeOf(&MCPDiscardInboxItemResponse{})
}

// Handle processes the MCP call tool request and returns the response
func (h *mcpDiscardInboxItemToolHandler) Handle(c *core.WebContext, callToolReq *MCPCallToolRequest, user *models.User, currentConfig *settings.Config, services MCPAvailableServices) (any, []*MCPTextContent, error) {
	var request MCPDiscardInboxItemRequest

	if err := unmarshalToolArguments(callToolReq, &request); err != nil {
		return nil, nil, err
	}

	itemId, err := parseInboxItemIdArgument(request.Id)

	if err != nil {
		return nil, nil, err
	}

	if strings.TrimSpace(request.Reason) == "" {
		return nil, nil, errs.NewInboxToolError("reason is required")
	}

	err = services.GetInboxItemService().DiscardInboxItem(c, user.Uid, itemId, strings.TrimSpace(request.Reason))

	if err != nil {
		log.Warnf(c, "[discard_inbox_item_tool_handler.Handle] failed to discard inbox item \"id:%d\" for user \"uid:%d\", because %s", itemId, user.Uid, err.Error())
		return nil, nil, err
	}

	return newJSONTextResponse(&MCPDiscardInboxItemResponse{
		Success: true,
		Id:      utils.Int64ToString(itemId),
		Status:  inboxStatusDiscardedName,
	})
}
