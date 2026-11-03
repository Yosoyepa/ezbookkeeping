// Package aitest contains a test harness (a temporary SQLite database, a local file storage and seeded accounts and categories) for the tests of the AI import tools
package aitest

import (
	"bytes"
	"path/filepath"
	"testing"

	"xorm.io/xorm"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/datastore"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/services"
	"github.com/mayswind/ezbookkeeping/pkg/settings"
	"github.com/mayswind/ezbookkeeping/pkg/storage"
	"github.com/mayswind/ezbookkeeping/pkg/uuid"
)

// Uid is the id of the user created by the harness
const Uid = int64(1001)

// Env represents a test environment with a seeded user
type Env struct {
	T      *testing.T
	Ctx    core.Context
	Config *settings.Config
	User   *models.User

	// ids of the seeded accounts
	Cash   int64
	Bank   int64
	Dollar int64
}

// MemoryFile is a multipart.File whose content is in memory
type MemoryFile struct {
	*bytes.Reader
}

// Close does nothing
func (f *MemoryFile) Close() error {
	return nil
}

// NewMemoryFile returns a multipart.File with the content
func NewMemoryFile(content []byte) *MemoryFile {
	return &MemoryFile{Reader: bytes.NewReader(content)}
}

// NewEnv creates the database, the storage and the seeded user, accounts, categories and tags
func NewEnv(t *testing.T) *Env {
	t.Helper()

	config := &settings.Config{
		DatabaseConfig: &settings.DatabaseConfig{
			DatabaseType: settings.Sqlite3DbType,
			DatabasePath: filepath.Join(t.TempDir(), "ezbookkeeping.db"),
		},
		UuidGeneratorType:             settings.InternalUuidGeneratorType,
		UuidServerId:                  1,
		StorageType:                   settings.LocalFileSystemObjectStorageType,
		LocalFileSystemPath:           filepath.Join(t.TempDir(), "storage"),
		EnableTransactionPictures:     true,
		MaxTransactionPictureFileSize: 10 * 1024 * 1024,
		SecretKey:                     "aitest-secret-key",
		RootUrl:                       "http://localhost:8080/",
	}

	previousStores := *datastore.Container
	previousConfig := settings.Container.GetCurrentConfig()
	t.Cleanup(func() {
		*datastore.Container = previousStores
		settings.SetCurrentConfig(previousConfig)
	})

	settings.SetCurrentConfig(config)

	if err := datastore.InitializeDataStore(config); err != nil {
		t.Fatal(err)
	}

	if err := uuid.InitializeUuidGenerator(config); err != nil {
		t.Fatal(err)
	}

	if err := storage.InitializeStorageContainer(config); err != nil {
		t.Fatal(err)
	}

	for _, structure := range []any{
		new(models.Account),
		new(models.TransactionCategory),
		new(models.TransactionTag),
		new(models.TransactionTagIndex),
		new(models.Transaction),
		new(models.TransactionPictureInfo),
		new(models.InboxItem),
	} {
		if err := datastore.Container.UserDataStore.SyncStructs(structure); err != nil {
			t.Fatal(err)
		}
	}

	env := &Env{
		T:      t,
		Ctx:    core.NewNullContext(),
		Config: config,
		User: &models.User{
			Uid:                  Uid,
			Username:             "aitest",
			DefaultCurrency:      "COP",
			TransactionEditScope: models.TRANSACTION_EDIT_SCOPE_ALL,
		},
	}

	env.seed()

	return env
}

// Insert saves the models directly in the database of the user
func (e *Env) Insert(beans ...any) {
	e.T.Helper()

	err := datastore.Container.UserDataStore.Choose(Uid).DoTransaction(e.Ctx, func(sess *xorm.Session) error {
		for _, bean := range beans {
			if _, err := sess.Insert(bean); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		e.T.Fatal(err)
	}
}

func (e *Env) seed() {
	e.Cash, e.Bank, e.Dollar = 1, 2, 3

	newAccount := func(id int64, name string, currency string, category models.AccountCategory) *models.Account {
		return &models.Account{AccountId: id, Uid: Uid, Category: category, Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Name: name, DisplayOrder: int32(id), Icon: 1, Color: "000000", Currency: currency}
	}

	newCategory := func(id int64, parentId int64, categoryType models.TransactionCategoryType, name string) *models.TransactionCategory {
		return &models.TransactionCategory{CategoryId: id, Uid: Uid, Type: categoryType, ParentCategoryId: parentId, Name: name, DisplayOrder: int32(id), Icon: 1, Color: "000000"}
	}

	e.Insert(
		newAccount(e.Cash, "Efectivo", "COP", models.ACCOUNT_CATEGORY_CASH),
		newAccount(e.Bank, "Bancolombia", "COP", models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT),
		newAccount(e.Dollar, "Dólares", "USD", models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT),

		newCategory(100, 0, models.CATEGORY_TYPE_EXPENSE, "Alimentación"),
		newCategory(101, 100, models.CATEGORY_TYPE_EXPENSE, "Restaurantes"),
		newCategory(102, 100, models.CATEGORY_TYPE_EXPENSE, "Mercado"),
		newCategory(110, 0, models.CATEGORY_TYPE_EXPENSE, "Transporte"),
		newCategory(111, 110, models.CATEGORY_TYPE_EXPENSE, "Taxi"),
		newCategory(112, 110, models.CATEGORY_TYPE_EXPENSE, "Otros"),
		newCategory(113, 100, models.CATEGORY_TYPE_EXPENSE, "Otros"),
		newCategory(200, 0, models.CATEGORY_TYPE_INCOME, "Ingresos"),
		newCategory(201, 200, models.CATEGORY_TYPE_INCOME, "Salario"),
		newCategory(300, 0, models.CATEGORY_TYPE_TRANSFER, "Transferencias"),
		newCategory(301, 300, models.CATEGORY_TYPE_TRANSFER, "Transferencia"),

		&models.TransactionTag{TagId: 500, Uid: Uid, Name: "comida", DisplayOrder: 1},
		&models.TransactionTag{TagId: 501, Uid: Uid, Name: "viaje", DisplayOrder: 2},
	)
}

// Services gives the services singletons to the code under test, it implements aiimport.Services and mcp.MCPAvailableServices
type Services struct{}

// GetTransactionService returns the transaction service
func (Services) GetTransactionService() *services.TransactionService { return services.Transactions }

// GetTransactionCategoryService returns the transaction category service
func (Services) GetTransactionCategoryService() *services.TransactionCategoryService {
	return services.TransactionCategories
}

// GetTransactionTagService returns the transaction tag service
func (Services) GetTransactionTagService() *services.TransactionTagService {
	return services.TransactionTags
}

// GetAccountService returns the account service
func (Services) GetAccountService() *services.AccountService { return services.Accounts }

// GetUserService returns the user service
func (Services) GetUserService() *services.UserService { return services.Users }

// GetInboxItemService returns the inbox item service
func (Services) GetInboxItemService() *services.InboxItemService { return services.InboxItems }

// GetTransactionPictureService returns the transaction picture service
func (Services) GetTransactionPictureService() *services.TransactionPictureService {
	return services.TransactionPictures
}

// FakeJpeg is the content used as the image of the receipts in the tests
var FakeJpeg = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0xFF, 0xD9}

// NewReceipt uploads a receipt to the inbox of the user
func (e *Env) NewReceipt(note string) *models.InboxItem {
	e.T.Helper()

	item := &models.InboxItem{Uid: Uid, FileExtension: "jpg", OriginalFileName: "recibo.jpg", Note: note, CreatedIp: "127.0.0.1"}

	if err := services.InboxItems.CreateReceiptItem(e.Ctx, item, NewMemoryFile(FakeJpeg)); err != nil {
		e.T.Fatal(err)
	}

	return item
}

// NewStatement uploads a statement to the inbox of the user
func (e *Env) NewStatement(extension string, content []byte) *models.InboxItem {
	e.T.Helper()

	item := &models.InboxItem{Uid: Uid, FileExtension: extension, OriginalFileName: "extracto." + extension, CreatedIp: "127.0.0.1"}

	if err := services.InboxItems.CreateStatementItem(e.Ctx, item, NewMemoryFile(content)); err != nil {
		e.T.Fatal(err)
	}

	return item
}

// CountTransactions returns the count of the transactions of the user
func (e *Env) CountTransactions() int64 {
	e.T.Helper()

	count, err := services.Transactions.GetTotalTransactionCountByUid(e.Ctx, Uid)

	if err != nil {
		e.T.Fatal(err)
	}

	return count
}

// AccountBalance returns the balance of the account in hundredths
func (e *Env) AccountBalance(accountId int64) int64 {
	e.T.Helper()

	account, err := services.Accounts.GetAccountByAccountId(e.Ctx, Uid, accountId)

	if err != nil {
		e.T.Fatal(err)
	}

	return account.Balance
}
