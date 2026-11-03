package mcp

import (
	"encoding/json/v2"
	"reflect"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// MCPGetReceiptImageRequest represents all parameters of the get receipt image request
type MCPGetReceiptImageRequest struct {
	PictureId string `json:"picture_id" jsonschema_description:"Id of the transaction picture (receipt image) to get"`
}

// MCPGetReceiptImageResponse represents the response structure for getting a receipt image
type MCPGetReceiptImageResponse struct {
	PictureId     string `json:"picture_id" jsonschema_description:"Id of the transaction picture"`
	TransactionId string `json:"transaction_id,omitempty" jsonschema_description:"Id of the transaction the picture is attached to (omitted when the picture is not attached to any transaction yet)"`
	UploadedTime  string `json:"uploaded_time" jsonschema_description:"Time the picture was uploaded in RFC 3339 format (e.g. 2023-01-01T12:00:00Z)"`
	MimeType      string `json:"mime_type" jsonschema_description:"MIME type of the image returned in the content"`
}

type mcpGetReceiptImageToolHandler struct{}

var MCPGetReceiptImageToolHandler = &mcpGetReceiptImageToolHandler{}

// ToolType returns the MCP tool type
func (h *mcpGetReceiptImageToolHandler) ToolType() core.MCPToolType {
	return core.MCP_TOOL_TYPE_GET_RECEIPT_IMAGE
}

// Name returns the name of the MCP tool
func (h *mcpGetReceiptImageToolHandler) Name() string {
	return "get_receipt_image"
}

// Description returns the description of the MCP tool
func (h *mcpGetReceiptImageToolHandler) Description() string {
	return "Get the image of a receipt (transaction picture) of the current user by its picture id."
}

// InputType returns the input type for the MCP tool request
func (h *mcpGetReceiptImageToolHandler) InputType() reflect.Type {
	return reflect.TypeOf(&MCPGetReceiptImageRequest{})
}

// OutputType returns the output type for the MCP tool response
func (h *mcpGetReceiptImageToolHandler) OutputType() reflect.Type {
	return reflect.TypeOf(&MCPGetReceiptImageResponse{})
}

// Handle processes the MCP call tool request and returns the response
func (h *mcpGetReceiptImageToolHandler) Handle(c *core.WebContext, callToolReq *MCPCallToolRequest, user *models.User, currentConfig *settings.Config, services MCPAvailableServices) (any, []*MCPImageContent, error) {
	var getReceiptImageRequest MCPGetReceiptImageRequest

	if callToolReq.Arguments != nil {
		if err := json.Unmarshal(callToolReq.Arguments, &getReceiptImageRequest); err != nil {
			return nil, nil, errs.NewIncompleteOrIncorrectSubmissionError(err)
		}
	} else {
		return nil, nil, errs.ErrIncompleteOrIncorrectSubmission
	}

	if !currentConfig.EnableTransactionPictures {
		return nil, nil, errs.ErrNotPermittedToPerformThisAction
	}

	pictureId, err := utils.StringToInt64(getReceiptImageRequest.PictureId)

	if err != nil || pictureId <= 0 {
		return nil, nil, errs.ErrTransactionPictureIdInvalid
	}

	uid := user.Uid
	pictureService := services.GetTransactionPictureService()

	// both queries are scoped to the uid of the token, so a picture of another user is reported as not found
	pictureInfo, err := pictureService.GetPictureInfoByPictureId(c, uid, pictureId)

	if err != nil {
		log.Warnf(c, "[get_receipt_image_tool_handler.Handle] failed to get picture info \"id:%d\" for user \"uid:%d\", because %s", pictureId, uid, err.Error())
		return nil, nil, err
	}

	mimeType := utils.GetImageContentType(pictureInfo.PictureExtension)

	if mimeType == "" {
		return nil, nil, errs.ErrImageTypeNotSupported
	}

	pictureData, err := pictureService.GetPictureByPictureId(c, uid, pictureId, pictureInfo.PictureExtension)

	if err != nil {
		log.Errorf(c, "[get_receipt_image_tool_handler.Handle] failed to get picture data \"id:%d\" for user \"uid:%d\", because %s", pictureId, uid, err.Error())
		return nil, nil, err
	}

	response := &MCPGetReceiptImageResponse{
		PictureId:    utils.Int64ToString(pictureInfo.PictureId),
		UploadedTime: time.Unix(pictureInfo.CreatedUnixTime, 0).UTC().Format(time.RFC3339),
		MimeType:     mimeType,
	}

	if pictureInfo.TransactionId > 0 {
		response.TransactionId = utils.Int64ToString(pictureInfo.TransactionId)
	}

	return response, []*MCPImageContent{
		NewMCPImageContent(pictureData, mimeType),
	}, nil
}
