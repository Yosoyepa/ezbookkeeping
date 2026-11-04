package aiimport

import (
	"fmt"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/log"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// TransactionInfo represents the main data of a transaction, with names instead of ids
type TransactionInfo struct {
	Id                     string   `json:"id" jsonschema_description:"Transaction id"`
	Time                   string   `json:"time" jsonschema_description:"Transaction time in RFC 3339 format"`
	Type                   string   `json:"type" jsonschema:"enum=income,enum=expense,enum=transfer" jsonschema_description:"Transaction type"`
	Amount                 string   `json:"amount" jsonschema_description:"Amount of the transaction"`
	Currency               string   `json:"currency,omitempty" jsonschema_description:"Currency of the account"`
	CategoryName           string   `json:"category_name,omitempty" jsonschema_description:"Secondary category name"`
	AccountName            string   `json:"account_name,omitempty" jsonschema_description:"Account name"`
	DestinationAccountName string   `json:"destination_account_name,omitempty" jsonschema_description:"Destination account name (transfers only)"`
	DestinationAmount      string   `json:"destination_amount,omitempty" jsonschema_description:"Destination amount (transfers only)"`
	Tags                   []string `json:"tags" jsonschema_description:"Names of the tags"`
	Comment                string   `json:"comment,omitempty" jsonschema_description:"Description of the transaction"`
}

// UpdateOptions represents the changes to apply to a transaction, nil fields are not changed
type UpdateOptions struct {
	TransactionId int64
	CategoryName  *string
	Tags          *[]string
	Comment       *string
	Amount        *string
	Time          *string
	AccountName   *string
	DryRun        bool
}

// UpdateResult represents the transaction before and after the update
type UpdateResult struct {
	Before  *TransactionInfo
	After   *TransactionInfo
	Changed bool
}

type updateLookups struct {
	accounts   map[int64]*models.Account
	accountMap map[string]*models.Account
	categories map[int64]*models.TransactionCategory
	allCats    []*models.TransactionCategory
	tagById    map[int64]*models.TransactionTag
	tagByName  map[string]*models.TransactionTag
}

// UpdateTransaction changes the category, tags, comment, amount, time or account of an existing transaction
func UpdateTransaction(c core.Context, svc Services, user *models.User, options *UpdateOptions) (*UpdateResult, error) {
	if options.CategoryName == nil && options.Tags == nil && options.Comment == nil && options.Amount == nil && options.Time == nil && options.AccountName == nil {
		return nil, errs.ErrIncompleteOrIncorrectSubmission
	}

	uid := user.Uid
	transaction, err := svc.GetTransactionService().GetTransactionByTransactionId(c, uid, options.TransactionId)

	if err != nil {
		return nil, err
	}

	if transaction.Type != models.TRANSACTION_DB_TYPE_INCOME && transaction.Type != models.TRANSACTION_DB_TYPE_EXPENSE && transaction.Type != models.TRANSACTION_DB_TYPE_TRANSFER_OUT {
		return nil, errs.ErrTransactionTypeInvalid
	}

	lookups, err := loadUpdateLookups(c, svc, uid)

	if err != nil {
		return nil, err
	}

	tagIdsMap, err := svc.GetTransactionTagService().GetAllTagIdsOfTransactions(c, uid, []int64{transaction.TransactionId})

	if err != nil {
		return nil, err
	}

	currentTagIds := tagIdsMap[transaction.TransactionId]
	before := lookups.toInfo(transaction, currentTagIds)

	newTransaction := *transaction
	newTagIds := currentTagIds

	if options.CategoryName != nil {
		categoryType := models.CATEGORY_TYPE_EXPENSE

		if transaction.Type == models.TRANSACTION_DB_TYPE_INCOME {
			categoryType = models.CATEGORY_TYPE_INCOME
		} else if transaction.Type == models.TRANSACTION_DB_TYPE_TRANSFER_OUT {
			categoryType = models.CATEGORY_TYPE_TRANSFER
		}

		category := lookups.findCategory(*options.CategoryName, categoryType)

		if category == nil {
			return nil, errs.NewInboxToolError(fmt.Sprintf("the category \"%s\" does not exist for this type of transaction, ask the user which category to use", *options.CategoryName))
		}

		newTransaction.CategoryId = category.CategoryId
	}

	if options.AccountName != nil {
		account, exists := lookups.accountMap[*options.AccountName]

		if !exists {
			return nil, errs.NewInboxToolError(fmt.Sprintf("the account \"%s\" does not exist, ask the user which account to use", *options.AccountName))
		}

		oldAccount := lookups.accounts[transaction.AccountId]

		if oldAccount != nil && oldAccount.Currency != account.Currency {
			return nil, errs.NewInboxToolError(fmt.Sprintf("the account \"%s\" uses another currency than the current account \"%s\"", account.Name, oldAccount.Name))
		}

		newTransaction.AccountId = account.AccountId
	}

	if options.Amount != nil {
		amount, err := ParseLocalizedAmount(*options.Amount)

		if err != nil {
			return nil, errs.NewInboxToolError(err.Error())
		}

		if amount <= 0 || amount > models.MaximumTransactionAmount {
			return nil, errs.NewInboxToolError("the amount must be greater than zero")
		}

		newTransaction.Amount = amount

		if transaction.Type == models.TRANSACTION_DB_TYPE_TRANSFER_OUT {
			source, destination := lookups.accounts[newTransaction.AccountId], lookups.accounts[newTransaction.RelatedAccountId]

			if source == nil || destination == nil || source.Currency != destination.Currency {
				return nil, errs.NewInboxToolError("the amount of a transfer between accounts with different currencies cannot be changed with this tool")
			}

			newTransaction.RelatedAccountAmount = amount
		}
	}

	if options.Time != nil {
		transactionTime, err := utils.ParseFromLongDateTimeWithTimezoneRFC3339Format(*options.Time)

		if err != nil {
			return nil, errs.NewInboxToolError(fmt.Sprintf("the time \"%s\" is invalid, use the RFC 3339 format (e.g. 2026-10-15T12:00:00-05:00)", *options.Time))
		}

		newTransaction.TransactionTime = utils.GetMinTransactionTimeFromUnixTime(transactionTime.Unix())
		newTransaction.TimezoneUtcOffset = utils.GetTimezoneOffsetMinutes(transactionTime.Unix(), transactionTime.Location())
	}

	if options.Comment != nil {
		newComment := *options.Comment
		source := SourceOfComment(transaction.Comment)

		if source != "" && !HasOriginSuffix(newComment) {
			newComment = BuildComment(newComment, source)
		} else {
			newComment = truncateString(newComment, maxCommentLength)
		}

		newTransaction.Comment = newComment
	}

	if options.Tags != nil {
		newTagIds = make([]int64, 0, len(*options.Tags))

		for _, tagName := range *options.Tags {
			tag, exists := lookups.tagByName[tagName]

			if !exists {
				return nil, errs.NewInboxToolError(fmt.Sprintf("the tag \"%s\" does not exist", tagName))
			}

			newTagIds = append(newTagIds, tag.TagId)
		}

		newTagIds = utils.ToUniqueInt64Slice(newTagIds)

		if len(newTagIds) > models.MaximumTagsCountOfTransaction {
			return nil, errs.ErrTransactionHasTooManyTags
		}
	}

	oldTimezone := time.FixedZone("Transaction Timezone", int(transaction.TimezoneUtcOffset)*60)
	newTimezone := time.FixedZone("Transaction Timezone", int(newTransaction.TimezoneUtcOffset)*60)

	if !user.CanEditTransactionByTransactionTime(transaction.TransactionTime, oldTimezone, lookups.accounts[transaction.AccountId], lookups.accounts[transaction.RelatedAccountId]) ||
		!user.CanEditTransactionByTransactionTime(newTransaction.TransactionTime, newTimezone, lookups.accounts[newTransaction.AccountId], lookups.accounts[newTransaction.RelatedAccountId]) {
		return nil, errs.ErrCannotModifyTransactionWithThisTransactionTime
	}

	result := &UpdateResult{
		Before: before,
	}

	addTagIds := utils.Int64SliceMinus(newTagIds, currentTagIds)
	removeTagIds := utils.Int64SliceMinus(currentTagIds, newTagIds)

	if options.DryRun {
		result.After = lookups.toInfo(&newTransaction, newTagIds)
		result.Changed = !sameInfo(result.Before, result.After)

		return result, nil
	}

	err = svc.GetTransactionService().ModifyTransaction(c, &newTransaction, false, len(currentTagIds), addTagIds, removeTagIds, nil, nil)

	if err != nil {
		log.Warnf(c, "[aiimport.UpdateTransaction] failed to modify transaction \"id:%d\" for user \"uid:%d\", because %s", options.TransactionId, uid, err.Error())
		return nil, err
	}

	result.After = lookups.toInfo(&newTransaction, newTagIds)
	result.Changed = true

	return result, nil
}

func loadUpdateLookups(c core.Context, svc Services, uid int64) (*updateLookups, error) {
	allAccounts, err := svc.GetAccountService().GetAllAccountsByUid(c, uid)

	if err != nil {
		return nil, err
	}

	allCategories, err := svc.GetTransactionCategoryService().GetAllCategoriesByUid(c, uid, 0, -1)

	if err != nil {
		return nil, err
	}

	allTags, err := svc.GetTransactionTagService().GetAllTagsByUid(c, uid)

	if err != nil {
		return nil, err
	}

	return &updateLookups{
		accounts:   svc.GetAccountService().GetAccountMapByList(allAccounts),
		accountMap: svc.GetAccountService().GetVisibleAccountNameMapByList(allAccounts),
		categories: svc.GetTransactionCategoryService().GetCategoryMapByList(allCategories),
		allCats:    allCategories,
		tagById:    svc.GetTransactionTagService().GetTagMapByList(allTags),
		tagByName:  svc.GetTransactionTagService().GetVisibleTagNameMapByList(allTags),
	}, nil
}

func (l *updateLookups) findCategory(name string, categoryType models.TransactionCategoryType) *models.TransactionCategory {
	for _, category := range l.allCats {
		if category.Hidden || category.ParentCategoryId == models.LevelOneTransactionCategoryParentId {
			continue
		}

		if category.Name == name && category.Type == categoryType {
			return category
		}
	}

	return nil
}

func (l *updateLookups) toInfo(transaction *models.Transaction, tagIds []int64) *TransactionInfo {
	info := &TransactionInfo{
		Id:     utils.Int64ToString(transaction.TransactionId),
		Amount: utils.FormatAmount(transaction.Amount),
		Tags:   make([]string, 0, len(tagIds)),
	}

	switch transaction.Type {
	case models.TRANSACTION_DB_TYPE_INCOME:
		info.Type = TransactionTypeIncome
	case models.TRANSACTION_DB_TYPE_EXPENSE:
		info.Type = TransactionTypeExpense
	case models.TRANSACTION_DB_TYPE_TRANSFER_OUT:
		info.Type = TransactionTypeTransfer
	}

	transactionTimezone := time.FixedZone("Transaction Timezone", int(transaction.TimezoneUtcOffset)*60)
	info.Time = utils.FormatUnixTimeToLongDateTimeWithTimezoneRFC3339Format(utils.GetUnixTimeFromTransactionTime(transaction.TransactionTime), transactionTimezone)
	info.Comment = transaction.Comment

	if account, exists := l.accounts[transaction.AccountId]; exists {
		info.AccountName = account.Name
		info.Currency = account.Currency
	}

	if category, exists := l.categories[transaction.CategoryId]; exists {
		info.CategoryName = category.Name
	}

	if transaction.Type == models.TRANSACTION_DB_TYPE_TRANSFER_OUT {
		info.DestinationAmount = utils.FormatAmount(transaction.RelatedAccountAmount)

		if account, exists := l.accounts[transaction.RelatedAccountId]; exists {
			info.DestinationAccountName = account.Name
		}
	}

	for _, tagId := range tagIds {
		if tag, exists := l.tagById[tagId]; exists {
			info.Tags = append(info.Tags, tag.Name)
		}
	}

	return info
}

func sameInfo(a *TransactionInfo, b *TransactionInfo) bool {
	return a.Id == b.Id && a.Time == b.Time && a.Type == b.Type && a.Amount == b.Amount && a.Currency == b.Currency && a.CategoryName == b.CategoryName &&
		a.AccountName == b.AccountName && a.DestinationAccountName == b.DestinationAccountName && a.DestinationAmount == b.DestinationAmount &&
		a.Comment == b.Comment && sameStrings(a.Tags, b.Tags)
}

func sameStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func truncateString(str string, maxRunes int) string {
	runes := []rune(str)

	if len(runes) <= maxRunes {
		return str
	}

	return string(runes[:maxRunes])
}
