package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport"
	"github.com/mayswind/ezbookkeeping/pkg/aiimport/aitest"
	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

func newInboxTestContext(request *http.Request, params gin.Params) *core.WebContext {
	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = request
	ginContext.Params = params

	c := core.WrapWebContext(ginContext, nil)
	c.SetTokenClaims(&core.UserTokenClaims{Uid: aitest.Uid, Type: core.USER_TOKEN_TYPE_API})

	return c
}

func newUploadRequest(t *testing.T, fields map[string]string, fileName string, content []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for name, value := range fields {
		assert.Nil(t, writer.WriteField(name, value))
	}

	if fileName != "" {
		part, err := writer.CreateFormFile("file", fileName)
		assert.Nil(t, err)
		_, err = part.Write(content)
		assert.Nil(t, err)
	}

	assert.Nil(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/api/v1/inbox/items/upload.json", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	return request
}

func newJSONRequest(method string, target string, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	return request
}

func uploadForTest(t *testing.T, kind string, fileName string, content []byte, note string) *models.InboxItemInfoResponse {
	t.Helper()
	c := newInboxTestContext(newUploadRequest(t, map[string]string{"kind": kind, "note": note}, fileName, content), nil)

	result, err := InboxItems.InboxItemUploadHandler(c)
	assert.Nil(t, err)

	response, ok := result.(*models.InboxItemInfoResponse)
	assert.True(t, ok)

	return response
}

func TestInboxItemsApi_UploadReceiptAndStatement(t *testing.T) {
	aitest.NewEnv(t)

	receipt := uploadForTest(t, "1", "recibo.jpg", aitest.FakeJpeg, "recibo del Éxito")
	assert.Equal(t, models.INBOX_ITEM_KIND_RECEIPT, receipt.Kind)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PENDING, receipt.Status)
	assert.Equal(t, "jpg", receipt.FileExtension)
	assert.Equal(t, "recibo del Éxito", receipt.Note)
	assert.Equal(t, "http://localhost:8080/pictures/"+utils.Int64ToString(receipt.PictureId)+".jpg", receipt.PictureUrl)
	assert.NotEmpty(t, receipt.Id)
	assert.NotNil(t, receipt.ResultTransactionIds)
	assert.Equal(t, 0, len(receipt.ResultTransactionIds))
	assert.Greater(t, receipt.CreatedTime, int64(0))

	statement := uploadForTest(t, "2", "extracto.csv", []byte("fecha;valor\n"), "")
	assert.Equal(t, models.INBOX_ITEM_KIND_STATEMENT, statement.Kind)
	assert.Equal(t, int64(0), statement.PictureId)
	assert.Empty(t, statement.PictureUrl)

	// stats
	result, err := InboxItems.InboxItemStatsHandler(newInboxTestContext(httptest.NewRequest(http.MethodGet, "/", nil), nil))
	assert.Nil(t, err)
	assert.Equal(t, &models.InboxItemStatsResponse{PendingReceipts: 1, PendingStatements: 1}, result)

	// list
	listContext := newInboxTestContext(httptest.NewRequest(http.MethodGet, "/api/v1/inbox/items/list.json?status=1&count=10&page=1", nil), nil)
	result, err = InboxItems.InboxItemListHandler(listContext)
	assert.Nil(t, err)
	list := result.(*models.InboxItemListResponse)
	assert.Equal(t, int64(2), list.TotalCount)
	assert.Equal(t, 2, len(list.Items))
	assert.Equal(t, statement.Id, list.Items[0].Id) // newest first, or the highest id when they were created in the same second

	listContext = newInboxTestContext(httptest.NewRequest(http.MethodGet, "/api/v1/inbox/items/list.json?kind=1&count=10&page=1", nil), nil)
	result, err = InboxItems.InboxItemListHandler(listContext)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), result.(*models.InboxItemListResponse).TotalCount)

	// the list requires count and page
	_, err = InboxItems.InboxItemListHandler(newInboxTestContext(httptest.NewRequest(http.MethodGet, "/api/v1/inbox/items/list.json", nil), nil))
	assert.NotNil(t, err)

	// get
	result, err = InboxItems.InboxItemGetHandler(newInboxTestContext(httptest.NewRequest(http.MethodGet, "/api/v1/inbox/items/get.json?id="+idText(receipt), nil), nil))
	assert.Nil(t, err)
	assert.Equal(t, receipt.Id, result.(*models.InboxItemInfoResponse).Id)

	_, err = InboxItems.InboxItemGetHandler(newInboxTestContext(httptest.NewRequest(http.MethodGet, "/api/v1/inbox/items/get.json?id=123", nil), nil))
	assert.Equal(t, errs.ErrInboxItemNotFound, err)
}

func TestInboxItemsApi_UploadValidation(t *testing.T) {
	env := aitest.NewEnv(t)

	testCases := []struct {
		name     string
		fields   map[string]string
		fileName string
		content  []byte
		expected *errs.Error
	}{
		{"missing kind", map[string]string{}, "r.jpg", aitest.FakeJpeg, errs.ErrInboxItemKindInvalid},
		{"invalid kind", map[string]string{"kind": "3"}, "r.jpg", aitest.FakeJpeg, errs.ErrInboxItemKindInvalid},
		{"no file", map[string]string{"kind": "1"}, "", nil, errs.ErrNoInboxItemFile},
		{"empty file", map[string]string{"kind": "1"}, "r.jpg", []byte{}, errs.ErrInboxItemFileIsEmpty},
		{"receipt that is not an image", map[string]string{"kind": "1"}, "r.pdf", []byte("pdf"), errs.ErrImageTypeNotSupported},
		{"statement with a forbidden extension", map[string]string{"kind": "2"}, "x.exe", []byte("x"), errs.ErrInboxItemFileTypeNotAllowed},
		{"statement without extension", map[string]string{"kind": "2"}, "extracto", []byte("x"), errs.ErrInboxItemFileTypeNotAllowed},
	}

	for _, testCase := range testCases {
		c := newInboxTestContext(newUploadRequest(t, testCase.fields, testCase.fileName, testCase.content), nil)
		result, err := InboxItems.InboxItemUploadHandler(c)
		assert.Nil(t, result, testCase.name)
		assert.Equal(t, testCase.expected, err, testCase.name)
	}

	// the size limits
	env.Config.MaxTransactionPictureFileSize = 10
	c := newInboxTestContext(newUploadRequest(t, map[string]string{"kind": "1"}, "r.jpg", bytes.Repeat([]byte{1}, 11)), nil)
	_, err := InboxItems.InboxItemUploadHandler(c)
	assert.Equal(t, errs.ErrExceedMaxTransactionPictureFileSize, err)

	c = newInboxTestContext(newUploadRequest(t, map[string]string{"kind": "2"}, "x.pdf", bytes.Repeat([]byte{1}, models.InboxItemStatementMaxFileSize+1)), nil)
	_, err = InboxItems.InboxItemUploadHandler(c)
	assert.Equal(t, errs.ErrExceedMaxInboxItemFileSize, err)

	// the inbox needs the storage of the pictures
	env.Config.EnableTransactionPictures = false
	c = newInboxTestContext(newUploadRequest(t, map[string]string{"kind": "2"}, "x.pdf", []byte("x")), nil)
	_, err = InboxItems.InboxItemUploadHandler(c)
	assert.Equal(t, errs.ErrInboxItemStorageDisabled, err)
}

func TestInboxItemsApi_DiscardReopenDelete(t *testing.T) {
	aitest.NewEnv(t)
	receipt := uploadForTest(t, "1", "r.jpg", aitest.FakeJpeg, "")

	result, err := InboxItems.InboxItemDiscardHandler(newInboxTestContext(newJSONRequest(http.MethodPost, "/", `{"id":"`+idText(receipt)+`","note":"duplicado"}`), nil))
	assert.Nil(t, err)
	assert.Equal(t, true, result)

	result, err = InboxItems.InboxItemGetHandler(newInboxTestContext(httptest.NewRequest(http.MethodGet, "/?id="+idText(receipt), nil), nil))
	assert.Nil(t, err)
	assert.Equal(t, models.INBOX_ITEM_STATUS_DISCARDED, result.(*models.InboxItemInfoResponse).Status)
	assert.Equal(t, "duplicado", result.(*models.InboxItemInfoResponse).ProcessedNote)

	result, err = InboxItems.InboxItemReopenHandler(newInboxTestContext(newJSONRequest(http.MethodPost, "/", `{"id":"`+idText(receipt)+`"}`), nil))
	assert.Nil(t, err)
	assert.Equal(t, true, result)

	result, err = InboxItems.InboxItemDeleteHandler(newInboxTestContext(newJSONRequest(http.MethodPost, "/", `{"id":"`+idText(receipt)+`"}`), nil))
	assert.Nil(t, err)
	assert.Equal(t, true, result)

	_, err = InboxItems.InboxItemGetHandler(newInboxTestContext(httptest.NewRequest(http.MethodGet, "/?id="+idText(receipt), nil), nil))
	assert.Equal(t, errs.ErrInboxItemNotFound, err)

	// invalid bodies
	_, err = InboxItems.InboxItemDiscardHandler(newInboxTestContext(newJSONRequest(http.MethodPost, "/", `{}`), nil))
	assert.NotNil(t, err)
	_, err = InboxItems.InboxItemDeleteHandler(newInboxTestContext(newJSONRequest(http.MethodPost, "/", `{"id":"0"}`), nil))
	assert.NotNil(t, err)
}

func TestInboxItemsApi_FileDownloads(t *testing.T) {
	env := aitest.NewEnv(t)
	receipt := uploadForTest(t, "1", "r.jpg", aitest.FakeJpeg, "")
	statement := uploadForTest(t, "2", "extracto.pdf", []byte("%PDF-1.4"), "")

	data, contentType, err := InboxItems.InboxItemFileGetHandler(newInboxTestContext(httptest.NewRequest(http.MethodGet, "/", nil), gin.Params{{Key: "id", Value: idText(receipt)}}))
	assert.Nil(t, err)
	assert.Equal(t, aitest.FakeJpeg, data)
	assert.Equal(t, "image/jpeg", contentType)

	data, contentType, err = InboxItems.InboxItemFileGetHandler(newInboxTestContext(httptest.NewRequest(http.MethodGet, "/", nil), gin.Params{{Key: "id", Value: idText(statement)}}))
	assert.Nil(t, err)
	assert.Equal(t, []byte("%PDF-1.4"), data)
	assert.Equal(t, "application/pdf", contentType)

	_, _, err = InboxItems.InboxItemFileGetHandler(newInboxTestContext(httptest.NewRequest(http.MethodGet, "/", nil), gin.Params{{Key: "id", Value: "abc"}}))
	assert.Equal(t, errs.ErrInboxItemIdInvalid, err)

	// the download by token needs no session, only a valid and unused token
	itemId := statement.Id
	token, tokenErr := aiimport.CreateDownloadToken(env.Config.SecretKey, aitest.Uid, itemId, time.Now().Add(aiimport.DownloadTokenLifetime))
	assert.Nil(t, tokenErr)

	anonymous := func() *core.WebContext {
		ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
		ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		ginContext.Params = gin.Params{{Key: "token", Value: token}}

		return core.WrapWebContext(ginContext, nil)
	}

	data, contentType, err = InboxItems.InboxItemFileDownloadByTokenHandler(anonymous())
	assert.Nil(t, err)
	assert.Equal(t, []byte("%PDF-1.4"), data)
	assert.Equal(t, "application/pdf", contentType)

	_, _, err = InboxItems.InboxItemFileDownloadByTokenHandler(anonymous())
	assert.Equal(t, errs.ErrInboxDownloadTokenInvalid, err)

	expired, _ := aiimport.CreateDownloadToken(env.Config.SecretKey, aitest.Uid, itemId, time.Now().Add(-time.Minute))
	token = expired
	_, _, err = InboxItems.InboxItemFileDownloadByTokenHandler(anonymous())
	assert.Equal(t, errs.ErrInboxDownloadTokenInvalid, err)

	token = url.QueryEscape("garbage")
	_, _, err = InboxItems.InboxItemFileDownloadByTokenHandler(anonymous())
	assert.Equal(t, errs.ErrInboxDownloadTokenInvalid, err)
}

func idText(response *models.InboxItemInfoResponse) string {
	return utils.Int64ToString(response.Id)
}
