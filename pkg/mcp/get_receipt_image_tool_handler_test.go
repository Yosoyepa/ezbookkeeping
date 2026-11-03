package mcp

import (
	"bytes"
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/datastore"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/services"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
	"github.com/mayswind/ezbookkeeping/pkg/storage"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

type receiptImageTestServices struct {
	MCPAvailableServices
}

func (s *receiptImageTestServices) GetTransactionPictureService() *services.TransactionPictureService {
	return services.TransactionPictures
}

type receiptImageTestObject struct {
	*bytes.Reader
}

func (o *receiptImageTestObject) Close() error {
	return nil
}

func initializeReceiptImageTest(t *testing.T) (*core.WebContext, *settings.Config) {
	t.Helper()
	config := &settings.Config{
		DatabaseConfig: &settings.DatabaseConfig{
			DatabaseType: settings.Sqlite3DbType,
			DatabasePath: filepath.Join(t.TempDir(), "receipts.db"),
		},
		EnableTransactionPictures: true,
		StorageType:               settings.LocalFileSystemObjectStorageType,
		LocalFileSystemPath:       t.TempDir(),
	}

	previousDataStores := *datastore.Container
	previousStorages := *storage.Container
	t.Cleanup(func() {
		*datastore.Container = previousDataStores
		*storage.Container = previousStorages
	})

	if err := datastore.InitializeDataStore(config); err != nil {
		t.Fatal(err)
	}

	if err := datastore.Container.UserDataStore.SyncStructs(new(models.TransactionPictureInfo)); err != nil {
		t.Fatal(err)
	}

	if err := storage.InitializeStorageContainer(config); err != nil {
		t.Fatal(err)
	}

	return &core.WebContext{Context: &gin.Context{}}, config
}

func addReceiptImageForTest(t *testing.T, c *core.WebContext, info *models.TransactionPictureInfo, data []byte) {
	t.Helper()

	if err := services.TransactionPictures.SaveTransactionPicture(c, info.Uid, info.PictureId, &receiptImageTestObject{bytes.NewReader(data)}, info.PictureExtension); err != nil {
		t.Fatal(err)
	}

	if _, err := datastore.Container.UserDataStore.Choose(info.Uid).NewSession(c).Insert(info); err != nil {
		t.Fatal(err)
	}
}

func callReceiptImageTool(c *core.WebContext, config *settings.Config, uid int64, arguments string) (any, []*MCPImageContent, error) {
	request := &MCPCallToolRequest{Name: MCPGetReceiptImageToolHandler.Name()}

	if arguments != "" {
		request.Arguments = []byte(arguments)
	}

	return MCPGetReceiptImageToolHandler.Handle(c, request, &models.User{Uid: uid}, config, &receiptImageTestServices{})
}

func TestMCPGetReceiptImageToolHandler_ReturnsImageWithMimeType(t *testing.T) {
	c, config := initializeReceiptImageTest(t)

	extensions := map[string]string{
		"jpg":  "image/jpeg",
		"jpeg": "image/jpeg",
		"png":  "image/png",
		"webp": "image/webp",
	}
	pictureId := int64(1000)

	for extension, expectedMimeType := range extensions {
		pictureId++
		data := []byte("fake-" + extension + "-image-bytes")
		addReceiptImageForTest(t, c, &models.TransactionPictureInfo{
			Uid:              1,
			PictureId:        pictureId,
			TransactionId:    77,
			PictureExtension: extension,
			CreatedUnixTime:  1793145600, // 2026-10-28T00:00:00Z
		}, data)

		structured, contents, err := callReceiptImageTool(c, config, 1, `{"picture_id":"`+utils.Int64ToString(pictureId)+`"}`)

		assert.Nil(t, err, extension)
		assert.Len(t, contents, 1, extension)
		assert.Equal(t, "image", contents[0].Type, extension)
		assert.Equal(t, expectedMimeType, contents[0].MimeType, extension)
		assert.Equal(t, base64.StdEncoding.EncodeToString(data), contents[0].Data, extension)

		response, ok := structured.(*MCPGetReceiptImageResponse)
		assert.True(t, ok, extension)
		assert.Equal(t, utils.Int64ToString(pictureId), response.PictureId, extension)
		assert.Equal(t, "77", response.TransactionId, extension)
		assert.Equal(t, "2026-10-28T00:00:00Z", response.UploadedTime, extension)
		assert.Equal(t, expectedMimeType, response.MimeType, extension)
	}
}

func TestMCPGetReceiptImageToolHandler_UnattachedPictureOmitsTransactionId(t *testing.T) {
	c, config := initializeReceiptImageTest(t)
	addReceiptImageForTest(t, c, &models.TransactionPictureInfo{
		Uid:              1,
		PictureId:        2001,
		TransactionId:    models.TransactionPictureNewPictureTransactionId,
		PictureExtension: "png",
		CreatedUnixTime:  1793145600,
	}, []byte("png"))

	structured, _, err := callReceiptImageTool(c, config, 1, `{"picture_id":"2001"}`)

	assert.Nil(t, err)
	assert.Equal(t, "", structured.(*MCPGetReceiptImageResponse).TransactionId)
}

func TestMCPGetReceiptImageToolHandler_PictureOfAnotherUser(t *testing.T) {
	c, config := initializeReceiptImageTest(t)
	addReceiptImageForTest(t, c, &models.TransactionPictureInfo{
		Uid:              1,
		PictureId:        3001,
		TransactionId:    5,
		PictureExtension: "jpg",
		CreatedUnixTime:  1793145600,
	}, []byte("jpg"))

	structured, contents, err := callReceiptImageTool(c, config, 2, `{"picture_id":"3001"}`)

	assert.Nil(t, structured)
	assert.Nil(t, contents)
	assert.Equal(t, errs.ErrTransactionPictureNotFound, err)
}

func TestMCPGetReceiptImageToolHandler_PictureNotFound(t *testing.T) {
	c, config := initializeReceiptImageTest(t)

	_, contents, err := callReceiptImageTool(c, config, 1, `{"picture_id":"999999"}`)

	assert.Nil(t, contents)
	assert.Equal(t, errs.ErrTransactionPictureNotFound, err)
}

func TestMCPGetReceiptImageToolHandler_FileMissingInStorage(t *testing.T) {
	c, config := initializeReceiptImageTest(t)

	// the database row exists but the image file was never saved
	_, err := datastore.Container.UserDataStore.Choose(1).NewSession(c).Insert(&models.TransactionPictureInfo{
		Uid:              1,
		PictureId:        4001,
		TransactionId:    5,
		PictureExtension: "jpg",
		CreatedUnixTime:  1793145600,
	})
	assert.Nil(t, err)

	_, contents, err := callReceiptImageTool(c, config, 1, `{"picture_id":"4001"}`)

	assert.Nil(t, contents)
	assert.Equal(t, errs.ErrTransactionPictureNoExists, err)
}

func TestMCPGetReceiptImageToolHandler_InvalidArguments(t *testing.T) {
	c, config := initializeReceiptImageTest(t)

	_, _, err := callReceiptImageTool(c, config, 1, "")
	assert.Equal(t, errs.ErrIncompleteOrIncorrectSubmission, err)

	for _, arguments := range []string{`{}`, `{"picture_id":""}`, `{"picture_id":"abc"}`, `{"picture_id":"0"}`, `{"picture_id":"-5"}`} {
		_, contents, err := callReceiptImageTool(c, config, 1, arguments)
		assert.Nil(t, contents, arguments)
		assert.Equal(t, errs.ErrTransactionPictureIdInvalid, err, arguments)
	}

	_, _, err = callReceiptImageTool(c, config, 1, `{"picture_id":12}`)
	assert.NotNil(t, err)
}

func TestMCPGetReceiptImageToolHandler_PicturesDisabled(t *testing.T) {
	c, config := initializeReceiptImageTest(t)
	config.EnableTransactionPictures = false

	_, contents, err := callReceiptImageTool(c, config, 1, `{"picture_id":"1"}`)

	assert.Nil(t, contents)
	assert.Equal(t, errs.ErrNotPermittedToPerformThisAction, err)
}
