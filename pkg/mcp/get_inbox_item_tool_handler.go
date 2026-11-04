package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport"
	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// maxEmbeddedInboxFileSize is the max size of a binary file (pdf, xlsx) embedded in the response, bigger files can only be downloaded
const maxEmbeddedInboxFileSize = 10 * 1024 * 1024

// inboxItemTextSeparator is the line between the metadata and the content in the text block of get_inbox_item
const inboxItemTextSeparator = "---"

// MCPGetInboxItemRequest represents all parameters of the get inbox item request
type MCPGetInboxItemRequest struct {
	Id string `json:"id" jsonschema_description:"Inbox item id (from list_inbox_items)"`
}

// MCPGetInboxItemResponse represents the metadata of an inbox item, the file itself is returned in the content blocks of the response
type MCPGetInboxItemResponse struct {
	Id                   string   `json:"id" jsonschema_description:"Inbox item id"`
	Kind                 string   `json:"kind" jsonschema:"enum=receipt,enum=statement" jsonschema_description:"receipt: the image follows in the response. statement: text files follow as text, pdf and spreadsheets as an embedded resource"`
	Status               string   `json:"status" jsonschema:"enum=pending,enum=processed,enum=discarded" jsonschema_description:"Status of the item"`
	CreatedTime          string   `json:"created_time" jsonschema_description:"Upload time in RFC 3339 format (UTC)"`
	Note                 string   `json:"note,omitempty" jsonschema_description:"Note written by the user when uploading"`
	OriginalFileName     string   `json:"original_file_name,omitempty" jsonschema_description:"Name of the uploaded file"`
	FileExtension        string   `json:"file_extension" jsonschema_description:"Extension of the file"`
	MimeType             string   `json:"mime_type,omitempty" jsonschema_description:"Content type of the file"`
	SizeBytes            int      `json:"size_bytes" jsonschema_description:"Size of the file in bytes"`
	DownloadUrl          string   `json:"download_url,omitempty" jsonschema_description:"Temporary url (valid 15 minutes, single use) to download the statement file, e.g. with curl, when the client cannot read the embedded resource"`
	ContentIncluded      bool     `json:"content_included" jsonschema_description:"Whether the file content is included in this response"`
	ContentNote          string   `json:"content_note,omitempty" jsonschema_description:"Why the file content is not included in this response (a pdf or spreadsheet bigger than 10 MB can only be downloaded with download_url)"`
	ResultTransactionIds []string `json:"result_transaction_ids,omitempty" jsonschema_description:"Ids of the transactions created from this item"`
	ProcessedNote        string   `json:"processed_note,omitempty" jsonschema_description:"Summary or reason saved when the item was processed or discarded"`
}

type mcpGetInboxItemToolHandler struct{}

var MCPGetInboxItemToolHandler = &mcpGetInboxItemToolHandler{}

// ToolType returns the MCP tool type
func (h *mcpGetInboxItemToolHandler) ToolType() core.MCPToolType {
	return core.MCP_TOOL_TYPE_GET_INBOX_ITEM
}

// Name returns the name of the MCP tool
func (h *mcpGetInboxItemToolHandler) Name() string {
	return "get_inbox_item"
}

// Description returns the description of the MCP tool
func (h *mcpGetInboxItemToolHandler) Description() string {
	return "Get an item of the inbox with its file. The text of the response is the metadata as json (id, kind, original_file_name, size_bytes...), and for a statement a line with --- and then: the complete text of a csv/txt/ofx/qif statement, or a note for a pdf/xlsx statement. A receipt also returns the image; a pdf/xlsx statement also returns an embedded resource (only up to 10 MB, bigger files only have the temporary download_url). Read the amounts, date and merchant only from the file, never guess them."
}

// InputType returns the input type for the MCP tool request
func (h *mcpGetInboxItemToolHandler) InputType() reflect.Type {
	return reflect.TypeOf(&MCPGetInboxItemRequest{})
}

// OutputType returns the output type for the MCP tool response
func (h *mcpGetInboxItemToolHandler) OutputType() reflect.Type {
	return reflect.TypeOf(&MCPGetInboxItemResponse{})
}

// Handle processes the MCP call tool request and returns the response
func (h *mcpGetInboxItemToolHandler) Handle(c *core.WebContext, callToolReq *MCPCallToolRequest, user *models.User, currentConfig *settings.Config, services MCPAvailableServices) (any, []*MCPMixedContent, error) {
	var request MCPGetInboxItemRequest

	if err := unmarshalToolArguments(callToolReq, &request); err != nil {
		return nil, nil, err
	}

	itemId, err := parseInboxItemIdArgument(request.Id)

	if err != nil {
		return nil, nil, err
	}

	item, err := services.GetInboxItemService().GetInboxItemByItemId(c, user.Uid, itemId)

	if err != nil {
		return nil, nil, err
	}

	data, mimeType, err := services.GetInboxItemService().ReadInboxItemFile(c, item)

	if err != nil {
		log.Errorf(c, "[get_inbox_item_tool_handler.Handle] failed to read the file of inbox item \"id:%d\" for user \"uid:%d\", because %s", itemId, user.Uid, err.Error())
		return nil, nil, err
	}

	response := &MCPGetInboxItemResponse{
		Id:               utils.Int64ToString(item.ItemId),
		Kind:             getInboxKindName(item.Kind),
		Status:           getInboxStatusName(item.Status),
		CreatedTime:      formatUnixTimeUTC(item.CreatedUnixTime),
		Note:             item.Note,
		OriginalFileName: item.OriginalFileName,
		FileExtension:    item.FileExtension,
		MimeType:         mimeType,
		SizeBytes:        len(data),
		ProcessedNote:    item.ProcessedNote,
	}

	for _, id := range item.GetResultTransactionIds() {
		response.ResultTransactionIds = append(response.ResultTransactionIds, utils.Int64ToString(id))
	}

	if item.Kind == models.INBOX_ITEM_KIND_STATEMENT {
		response.DownloadUrl = h.createDownloadUrl(currentConfig, user.Uid, itemId)
	}

	// The text of the response is a single block: the metadata and, after a line with ---, the content of a text statement or a note.
	// Some clients (Claude Code) only deliver the first text block of a response to the model, so nothing the model needs goes in a second text block.
	var fileBlock *MCPMixedContent
	textAfterMetadata := ""

	if item.Kind == models.INBOX_ITEM_KIND_RECEIPT {
		fileBlock = NewMCPMixedImageContent(base64.StdEncoding.EncodeToString(data), mimeType)
		response.ContentIncluded = true
	} else if isTextStatement(item.FileExtension) {
		textAfterMetadata = decodeStatementText(data)
		response.ContentIncluded = true
	} else if len(data) <= maxEmbeddedInboxFileSize {
		fileBlock = NewMCPMixedBlobResourceContent(fmt.Sprintf("inbox://items/%d/%s", itemId, h.getResourceFileName(item)), mimeType, base64.StdEncoding.EncodeToString(data))
		response.ContentIncluded = true
		textAfterMetadata = h.getEmbeddedNote(response)
	} else {
		response.ContentNote = h.getNotEmbeddedNote(response)
		textAfterMetadata = response.ContentNote
	}

	content, err := json.Marshal(response)

	if err != nil {
		return nil, nil, err
	}

	text := string(content)

	if item.Kind != models.INBOX_ITEM_KIND_RECEIPT {
		text += "\n" + inboxItemTextSeparator + "\n" + textAfterMetadata
	}

	blocks := []*MCPMixedContent{NewMCPMixedTextContent(text)}

	if fileBlock != nil {
		blocks = append(blocks, fileBlock)
	}

	return response, blocks, nil
}

// getEmbeddedNote tells what to do when the client cannot read the embedded file
func (h *mcpGetInboxItemToolHandler) getEmbeddedNote(response *MCPGetInboxItemResponse) string {
	note := "The file is attached to this response as an embedded resource."

	if response.DownloadUrl != "" {
		return note + " If you cannot read it, download it from download_url (valid 15 minutes, single use), e.g. with curl, and read it locally."
	}

	return note
}

// getNotEmbeddedNote explains why the file is not embedded in the response and how to get it
func (h *mcpGetInboxItemToolHandler) getNotEmbeddedNote(response *MCPGetInboxItemResponse) string {
	note := fmt.Sprintf("The file is %d bytes, bigger than the %d MB limit for embedding it in the response, so it is not included.", response.SizeBytes, maxEmbeddedInboxFileSize/(1024*1024))

	if response.DownloadUrl != "" {
		return note + " Download it from download_url (valid 15 minutes, single use), e.g. with curl, and read it locally."
	}

	return note + " The server has no root_url configured, so there is no download_url: download the file from the web app."
}

func (h *mcpGetInboxItemToolHandler) createDownloadUrl(config *settings.Config, uid int64, itemId int64) string {
	if config == nil || config.RootUrl == "" {
		return ""
	}

	token, err := aiimport.CreateDownloadToken(config.SecretKey, uid, itemId, time.Now().Add(aiimport.DownloadTokenLifetime))

	if err != nil {
		return ""
	}

	rootUrl := config.RootUrl

	if !strings.HasSuffix(rootUrl, "/") {
		rootUrl += "/"
	}

	return rootUrl + "inbox/download/" + token
}

func (h *mcpGetInboxItemToolHandler) getResourceFileName(item *models.InboxItem) string {
	if item.OriginalFileName != "" {
		return strings.ReplaceAll(item.OriginalFileName, "/", "_")
	}

	return fmt.Sprintf("%d.%s", item.ItemId, item.FileExtension)
}

func isTextStatement(fileExtension string) bool {
	switch fileExtension {
	case "csv", "txt", "ofx", "qif":
		return true
	}

	return false
}

// decodeStatementText returns the text of a statement file, bank files are often saved in Windows-1252 instead of UTF-8
func decodeStatementText(data []byte) string {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	if utf8.Valid(data) {
		return string(data)
	}

	decoded, err := charmap.Windows1252.NewDecoder().Bytes(data)

	if err != nil {
		return strings.ToValidUTF8(string(data), "?")
	}

	return string(decoded)
}
