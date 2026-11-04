package mcp

import (
	"reflect"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

const defaultListInboxItemsLimit = 20
const maximumListInboxItemsLimit = 50

// MCPListInboxItemsRequest represents all parameters of the list inbox items request
type MCPListInboxItemsRequest struct {
	Status string `json:"status,omitempty" jsonschema:"enum=pending,enum=processed,enum=discarded,enum=all,default=pending" jsonschema_description:"Status to filter by (optional, default pending)"`
	Kind   string `json:"kind,omitempty" jsonschema:"enum=receipt,enum=statement" jsonschema_description:"Kind to filter by (optional, default both)"`
	Limit  int32  `json:"limit,omitempty" jsonschema:"default=20" jsonschema_description:"Maximum number of items to return, newest first (optional, default 20, maximum 50)"`
}

// MCPInboxItemSummary defines the structure of an inbox item in a list
type MCPInboxItemSummary struct {
	Id               string `json:"id" jsonschema_description:"Inbox item id, use it with get_inbox_item and resolve_inbox_item"`
	Kind             string `json:"kind" jsonschema:"enum=receipt,enum=statement" jsonschema_description:"receipt: photo of a receipt. statement: bank statement document"`
	Status           string `json:"status" jsonschema:"enum=pending,enum=processed,enum=discarded" jsonschema_description:"Status of the item"`
	CreatedTime      string `json:"created_time" jsonschema_description:"Upload time in RFC 3339 format (UTC)"`
	Note             string `json:"note,omitempty" jsonschema_description:"Note written by the user when uploading"`
	OriginalFileName string `json:"original_file_name,omitempty" jsonschema_description:"Name of the uploaded file"`
}

// MCPListInboxItemsResponse represents the response structure for listing inbox items
type MCPListInboxItemsResponse struct {
	Items      []*MCPInboxItemSummary `json:"items" jsonschema_description:"Inbox items, newest first"`
	TotalCount int64                  `json:"total_count" jsonschema_description:"Total number of items matching the filters"`
}

type mcpListInboxItemsToolHandler struct{}

var MCPListInboxItemsToolHandler = &mcpListInboxItemsToolHandler{}

// ToolType returns the MCP tool type
func (h *mcpListInboxItemsToolHandler) ToolType() core.MCPToolType {
	return core.MCP_TOOL_TYPE_LIST_INBOX_ITEMS
}

// Name returns the name of the MCP tool
func (h *mcpListInboxItemsToolHandler) Name() string {
	return "list_inbox_items"
}

// Description returns the description of the MCP tool
func (h *mcpListInboxItemsToolHandler) Description() string {
	return "List the receipts (photos) and bank statements that the user uploaded to the ezBookkeeping inbox from the phone. By default only the pending ones are returned."
}

// InputType returns the input type for the MCP tool request
func (h *mcpListInboxItemsToolHandler) InputType() reflect.Type {
	return reflect.TypeOf(&MCPListInboxItemsRequest{})
}

// OutputType returns the output type for the MCP tool response
func (h *mcpListInboxItemsToolHandler) OutputType() reflect.Type {
	return reflect.TypeOf(&MCPListInboxItemsResponse{})
}

// Handle processes the MCP call tool request and returns the response
func (h *mcpListInboxItemsToolHandler) Handle(c *core.WebContext, callToolReq *MCPCallToolRequest, user *models.User, currentConfig *settings.Config, services MCPAvailableServices) (any, []*MCPTextContent, error) {
	var request MCPListInboxItemsRequest

	if callToolReq.Arguments != nil {
		if err := unmarshalToolArguments(callToolReq, &request); err != nil {
			return nil, nil, err
		}
	}

	status := models.INBOX_ITEM_STATUS_PENDING

	switch request.Status {
	case "", inboxStatusPendingName:
	case inboxStatusProcessedName:
		status = models.INBOX_ITEM_STATUS_PROCESSED
	case inboxStatusDiscardedName:
		status = models.INBOX_ITEM_STATUS_DISCARDED
	case "all":
		status = 0
	default:
		return nil, nil, errs.ErrInboxItemStatusInvalid
	}

	kind := models.InboxItemKind(0)

	switch request.Kind {
	case "":
	case inboxKindReceiptName:
		kind = models.INBOX_ITEM_KIND_RECEIPT
	case inboxKindStatementName:
		kind = models.INBOX_ITEM_KIND_STATEMENT
	default:
		return nil, nil, errs.ErrInboxItemKindInvalid
	}

	limit := request.Limit

	if limit <= 0 {
		limit = defaultListInboxItemsLimit
	} else if limit > maximumListInboxItemsLimit {
		limit = maximumListInboxItemsLimit
	}

	items, totalCount, err := services.GetInboxItemService().GetInboxItems(c, user.Uid, status, kind, 1, limit)

	if err != nil {
		log.Errorf(c, "[list_inbox_items_tool_handler.Handle] failed to get inbox items for user \"uid:%d\", because %s", user.Uid, err.Error())
		return nil, nil, err
	}

	response := &MCPListInboxItemsResponse{
		Items:      make([]*MCPInboxItemSummary, 0, len(items)),
		TotalCount: totalCount,
	}

	for _, item := range items {
		response.Items = append(response.Items, &MCPInboxItemSummary{
			Id:               utils.Int64ToString(item.ItemId),
			Kind:             getInboxKindName(item.Kind),
			Status:           getInboxStatusName(item.Status),
			CreatedTime:      formatUnixTimeUTC(item.CreatedUnixTime),
			Note:             item.Note,
			OriginalFileName: item.OriginalFileName,
		})
	}

	return newJSONTextResponse(response)
}
