package aiimport

import (
	"fmt"
	"strings"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/services"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// AITagName is the name of the tag added to every transaction created by the AI tools
const AITagName = "ia"

// Transaction types accepted in the rows
const (
	TransactionTypeIncome   = "income"
	TransactionTypeExpense  = "expense"
	TransactionTypeTransfer = "transfer"
)

// Status of a row after importing
const (
	RowStatusCreated     = "created"
	RowStatusWouldCreate = "would_create"
	RowStatusDuplicate   = "duplicate"
	RowStatusError       = "error"
)

// Services contains the services needed to import transactions
type Services interface {
	GetTransactionService() *services.TransactionService
	GetTransactionCategoryService() *services.TransactionCategoryService
	GetTransactionTagService() *services.TransactionTagService
	GetAccountService() *services.AccountService
	GetInboxItemService() *services.InboxItemService
}

// TxRow represents a transaction to create, it has the same fields as the add_transaction tool, but the amount accepts the Colombian format
type TxRow struct {
	Type                   string   `json:"type" jsonschema:"enum=income,enum=expense,enum=transfer" jsonschema_description:"Transaction type (income, expense, transfer)"`
	Time                   string   `json:"time" jsonschema:"format=date-time" jsonschema_description:"Transaction time in RFC 3339 format (e.g. 2026-10-15T12:00:00-05:00)"`
	SecondaryCategoryName  string   `json:"category_name" jsonschema_description:"Secondary category name for the transaction. It must exist, ask the user if none fits"`
	AccountName            string   `json:"account_name" jsonschema_description:"Account name for the transaction. It must exist, ask the user if none fits"`
	Amount                 string   `json:"amount" jsonschema_description:"Positive transaction amount as text. Colombian format is accepted (1.234.567,89 / $ 45.900), the response always shows the normalized amount"`
	DestinationAccountName string   `json:"destination_account_name,omitempty" jsonschema_description:"Destination account name for transfer transactions (optional)"`
	DestinationAmount      string   `json:"destination_amount,omitempty" jsonschema_description:"Destination amount for transfer transactions (optional)"`
	Tags                   []string `json:"tags,omitempty" jsonschema_description:"Names of existing tags (optional, maximum 9 tags because the tag 'ia' is always added)"`
	Comment                string   `json:"comment,omitempty" jsonschema_description:"Transaction description, usually the merchant. The origin suffix is added by the server"`
}

// RowResult represents the result of importing one row
type RowResult struct {
	Index            int    `json:"index" jsonschema_description:"Index of the row in the request, starting at 0"`
	Status           string `json:"status" jsonschema:"enum=created,enum=would_create,enum=duplicate,enum=error" jsonschema_description:"created: saved. would_create: valid, it would be saved (dry run). duplicate: skipped because a similar transaction exists. error: invalid row, see message"`
	TransactionId    string `json:"transaction_id,omitempty" jsonschema_description:"Id of the created transaction"`
	DuplicateOf      string `json:"duplicate_of,omitempty" jsonschema_description:"Id of the existing transaction or 'row:N' when the duplicate is another row of the same request"`
	Message          string `json:"message,omitempty" jsonschema_description:"Explanation for duplicate and error rows"`
	NormalizedAmount string `json:"normalized_amount,omitempty" jsonschema_description:"The amount as it was understood by the server (e.g. 45900.00), check it against the document"`
}

// Result represents the result of importing the rows
type Result struct {
	Created     int          `json:"created" jsonschema_description:"Count of transactions saved"`
	WouldCreate int          `json:"would_create,omitempty" jsonschema_description:"Count of rows that would be saved (dry run only)"`
	Duplicates  int          `json:"duplicates" jsonschema_description:"Count of rows skipped as duplicates"`
	Errors      int          `json:"errors" jsonschema_description:"Count of invalid rows"`
	Rows        []*RowResult `json:"rows" jsonschema_description:"Result of each row"`

	// CreatedTransactionIds contains the ids of the saved transactions in the order of the rows
	CreatedTransactionIds []int64 `json:"-"`
}

// Options represents the options of importing rows
type Options struct {
	SourceLabel    string
	SkipDuplicates bool
	DryRun         bool
	ClientIP       string

	// PictureId is the id of a picture not linked to any transaction, it is linked to the first transaction created
	PictureId int64
}

type acceptedRow struct {
	index     int
	accountId int64
	txType    models.TransactionDbType
	amount    int64
	unixTime  int64
	comment   string
}

type importContext struct {
	user          *models.User
	svc           Services
	accountMap    map[string]*models.Account
	categories    []*models.TransactionCategory
	tagMap        map[string]*models.TransactionTag
	aiTag         *models.TransactionTag
	aiTagCreated  bool
	accepted      []*acceptedRow
	pictureLinked bool
}

// Import validates the rows and creates the transactions, the result has one entry per row
func Import(c core.Context, svc Services, user *models.User, rows []*TxRow, options *Options) (*Result, error) {
	uid := user.Uid

	allAccounts, err := svc.GetAccountService().GetAllAccountsByUid(c, uid)

	if err != nil {
		log.Warnf(c, "[aiimport.Import] failed to get accounts of user \"uid:%d\", because %s", uid, err.Error())
		return nil, err
	}

	allCategories, err := svc.GetTransactionCategoryService().GetAllCategoriesByUid(c, uid, 0, -1)

	if err != nil {
		log.Warnf(c, "[aiimport.Import] failed to get categories of user \"uid:%d\", because %s", uid, err.Error())
		return nil, err
	}

	allTags, err := svc.GetTransactionTagService().GetAllTagsByUid(c, uid)

	if err != nil {
		log.Warnf(c, "[aiimport.Import] failed to get tags of user \"uid:%d\", because %s", uid, err.Error())
		return nil, err
	}

	ic := &importContext{
		user:       user,
		svc:        svc,
		accountMap: svc.GetAccountService().GetVisibleAccountNameMapByList(allAccounts),
		categories: allCategories,
		tagMap:     svc.GetTransactionTagService().GetVisibleTagNameMapByList(allTags),
	}

	for _, tag := range allTags {
		if tag.Name == AITagName {
			ic.aiTag = tag
			break
		}
	}

	result := &Result{
		Rows: make([]*RowResult, 0, len(rows)),
	}

	for i, row := range rows {
		rowResult := ic.importRow(c, i, row, options, result)
		result.Rows = append(result.Rows, rowResult)

		switch rowResult.Status {
		case RowStatusCreated:
			result.Created++
		case RowStatusWouldCreate:
			result.WouldCreate++
		case RowStatusDuplicate:
			result.Duplicates++
		default:
			result.Errors++
		}
	}

	return result, nil
}

func (ic *importContext) importRow(c core.Context, index int, row *TxRow, options *Options, result *Result) *RowResult {
	rowResult := &RowResult{
		Index: index,
	}

	fail := func(format string, args ...any) *RowResult {
		rowResult.Status = RowStatusError
		rowResult.Message = fmt.Sprintf(format, args...)
		return rowResult
	}

	if row == nil {
		return fail("the row is empty")
	}

	uid := ic.user.Uid

	var dbType models.TransactionDbType
	var categoryType models.TransactionCategoryType

	switch row.Type {
	case TransactionTypeExpense:
		dbType, categoryType = models.TRANSACTION_DB_TYPE_EXPENSE, models.CATEGORY_TYPE_EXPENSE
	case TransactionTypeIncome:
		dbType, categoryType = models.TRANSACTION_DB_TYPE_INCOME, models.CATEGORY_TYPE_INCOME
	case TransactionTypeTransfer:
		dbType, categoryType = models.TRANSACTION_DB_TYPE_TRANSFER_OUT, models.CATEGORY_TYPE_TRANSFER
	default:
		return fail("the type \"%s\" is invalid, it must be income, expense or transfer", row.Type)
	}

	transactionTime, err := utils.ParseFromLongDateTimeWithTimezoneRFC3339Format(row.Time)

	if err != nil {
		return fail("the time \"%s\" is invalid, use the RFC 3339 format (e.g. 2026-10-15T12:00:00-05:00)", row.Time)
	}

	amount, err := ParseLocalizedAmount(row.Amount)

	if err != nil {
		return fail("%s", err.Error())
	}

	rowResult.NormalizedAmount = utils.FormatAmount(amount)

	if amount <= 0 {
		return fail("the amount must be greater than zero, use the type (income or expense) to set the direction")
	}

	if amount > models.MaximumTransactionAmount {
		return fail("the amount \"%s\" is too large", row.Amount)
	}

	sourceAccount, exists := ic.accountMap[row.AccountName]

	if !exists {
		return fail("the account \"%s\" does not exist, ask the user which account to use", row.AccountName)
	}

	var destinationAccount *models.Account
	var destinationAmount int64

	if dbType == models.TRANSACTION_DB_TYPE_TRANSFER_OUT {
		destinationAccount, exists = ic.accountMap[row.DestinationAccountName]

		if !exists {
			return fail("the destination account \"%s\" does not exist", row.DestinationAccountName)
		}

		destinationAmount, err = ParseLocalizedAmount(row.DestinationAmount)

		if err != nil {
			return fail("destination amount: %s", err.Error())
		}

		if destinationAmount <= 0 || destinationAmount > models.MaximumTransactionAmount {
			return fail("the destination amount must be greater than zero")
		}
	}

	category := ic.findCategory(row.SecondaryCategoryName, categoryType)

	if category == nil {
		return fail("the %s category \"%s\" does not exist, ask the user which category to use", row.Type, row.SecondaryCategoryName)
	}

	tagIds := make([]int64, 0, len(row.Tags)+1)

	for _, tagName := range row.Tags {
		tag, exists := ic.tagMap[tagName]

		if !exists {
			return fail("the tag \"%s\" does not exist", tagName)
		}

		tagIds = append(tagIds, tag.TagId)
	}

	tagIds = utils.ToUniqueInt64Slice(tagIds)

	if ic.aiTag != nil {
		tagIds = utils.ToUniqueInt64Slice(append(tagIds, ic.aiTag.TagId))
	}

	tagCount := len(tagIds)

	if ic.aiTag == nil {
		tagCount++
	}

	if tagCount > models.MaximumTagsCountOfTransaction {
		return fail("the transaction has too many tags (maximum %d including the tag \"%s\")", models.MaximumTagsCountOfTransaction, AITagName)
	}

	transaction := &models.Transaction{
		Uid:               uid,
		Type:              dbType,
		CategoryId:        category.CategoryId,
		TransactionTime:   utils.GetMinTransactionTimeFromUnixTime(transactionTime.Unix()),
		TimezoneUtcOffset: utils.GetTimezoneOffsetMinutes(transactionTime.Unix(), transactionTime.Location()),
		AccountId:         sourceAccount.AccountId,
		Amount:            amount,
		Comment:           BuildComment(row.Comment, options.SourceLabel),
		CreatedIp:         options.ClientIP,
	}

	if destinationAccount != nil {
		transaction.RelatedAccountId = destinationAccount.AccountId
		transaction.RelatedAccountAmount = destinationAmount
	}

	transactionTimezone := time.FixedZone("Transaction Timezone", int(transaction.TimezoneUtcOffset)*60)

	if !ic.user.CanEditTransactionByTransactionTime(transaction.TransactionTime, transactionTimezone, sourceAccount, destinationAccount) {
		return fail("the time of the transaction is not editable according to the settings of the user")
	}

	if options.SkipDuplicates {
		duplicateOf, err := ic.findDuplicate(c, transaction, row.Comment, transactionTime.Unix())

		if err != nil {
			return fail("failed to check duplicates: %s", err.Error())
		}

		if duplicateOf != "" {
			rowResult.Status = RowStatusDuplicate
			rowResult.DuplicateOf = duplicateOf
			rowResult.Message = "a similar transaction already exists (same account, type and amount within 1 day, and a similar description); use skip_duplicates=false to create it anyway"
			return rowResult
		}
	}

	if options.DryRun {
		rowResult.Status = RowStatusWouldCreate
		ic.rememberAccepted(index, transaction, row.Comment, transactionTime.Unix())
		return rowResult
	}

	if ic.aiTag == nil {
		if err := ic.createAITag(c); err != nil {
			return fail("failed to create the tag \"%s\": %s", AITagName, err.Error())
		}

		tagIds = utils.ToUniqueInt64Slice(append(tagIds, ic.aiTag.TagId))
	}

	var pictureIds []int64

	if options.PictureId > 0 && !ic.pictureLinked {
		pictureIds = []int64{options.PictureId}
	}

	err = ic.svc.GetTransactionService().CreateTransaction(c, transaction, tagIds, pictureIds)

	if err != nil {
		log.Warnf(c, "[aiimport.importRow] failed to create transaction for user \"uid:%d\", because %s", uid, err.Error())
		return fail("failed to create the transaction: %s", err.Error())
	}

	if len(pictureIds) > 0 {
		ic.pictureLinked = true
	}

	ic.rememberAccepted(index, transaction, row.Comment, transactionTime.Unix())
	result.CreatedTransactionIds = append(result.CreatedTransactionIds, transaction.TransactionId)

	rowResult.Status = RowStatusCreated
	rowResult.TransactionId = utils.Int64ToString(transaction.TransactionId)

	return rowResult
}

func (ic *importContext) findCategory(name string, categoryType models.TransactionCategoryType) *models.TransactionCategory {
	for _, category := range ic.categories {
		if category.Hidden || category.ParentCategoryId == models.LevelOneTransactionCategoryParentId {
			continue
		}

		if category.Name == name && category.Type == categoryType {
			return category
		}
	}

	return nil
}

func (ic *importContext) createAITag(c core.Context) error {
	tagService := ic.svc.GetTransactionTagService()
	maxDisplayOrder, err := tagService.GetMaxDisplayOrder(c, ic.user.Uid, 0)

	if err != nil {
		return err
	}

	tag := &models.TransactionTag{
		Uid:          ic.user.Uid,
		Name:         AITagName,
		DisplayOrder: maxDisplayOrder + 1,
	}

	err = tagService.CreateTag(c, tag)

	if err != nil {
		return err
	}

	ic.aiTag = tag
	ic.aiTagCreated = true

	return nil
}

func (ic *importContext) rememberAccepted(index int, transaction *models.Transaction, comment string, unixTime int64) {
	ic.accepted = append(ic.accepted, &acceptedRow{
		index:     index,
		accountId: transaction.AccountId,
		txType:    transaction.Type,
		amount:    transaction.Amount,
		unixTime:  unixTime,
		comment:   comment,
	})
}

// findDuplicate returns the reference of a transaction (an id or "row:N") which is a duplicate of the transaction, or empty if there is none
func (ic *importContext) findDuplicate(c core.Context, transaction *models.Transaction, comment string, unixTime int64) (string, error) {
	for _, accepted := range ic.accepted {
		if accepted.accountId == transaction.AccountId && accepted.txType == transaction.Type && accepted.amount == transaction.Amount &&
			IsInDuplicateWindow(accepted.unixTime, unixTime) && CommentsMatch(accepted.comment, comment) {
			return fmt.Sprintf("row:%d", accepted.index), nil
		}
	}

	minTime := utils.GetMinTransactionTimeFromUnixTime(unixTime - duplicateWindowSeconds)
	maxTime := utils.GetMaxTransactionTimeFromUnixTime(unixTime + duplicateWindowSeconds)
	candidates, err := ic.svc.GetInboxItemService().FindDuplicateCandidates(c, ic.user.Uid, transaction.AccountId, transaction.Type, transaction.Amount, minTime, maxTime)

	if err != nil {
		return "", err
	}

	for _, candidate := range candidates {
		if CommentsMatch(candidate.Comment, comment) {
			return utils.Int64ToString(candidate.TransactionId), nil
		}
	}

	return "", nil
}

// FormatRowErrors returns a short text with the messages of the rows which failed
func FormatRowErrors(result *Result, maxRows int) string {
	messages := make([]string, 0)

	for _, row := range result.Rows {
		if row.Status != RowStatusError {
			continue
		}

		if len(messages) >= maxRows {
			messages = append(messages, "...")
			break
		}

		messages = append(messages, fmt.Sprintf("row %d: %s", row.Index, row.Message))
	}

	return strings.Join(messages, "; ")
}

// ValidateRowsCount returns an error when the count of rows is not in the range [1, max]
func ValidateRowsCount(count int, max int) error {
	if count < 1 || count > max {
		return errs.ErrIncompleteOrIncorrectSubmission
	}

	return nil
}
