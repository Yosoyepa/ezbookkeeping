package aiimport

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// Values of the group_by parameter of the spending summary
const (
	GroupByCategory         = "category"
	GroupByMonth            = "month"
	GroupByAccount          = "account"
	GroupByTag              = "tag"
	GroupByCommentMerchant  = "comment_merchant"
	merchantKeyMaxLength    = 40
	summaryOtherKey         = "(other)"
	summaryNoTagKey         = "(no tag)"
	summaryUnknownKey       = "(unknown)"
	defaultSummaryTopGroups = 20
)

// SummaryRow represents the total of one group of the spending summary
type SummaryRow struct {
	Key   string  `json:"key" jsonschema_description:"Name of the group (category, month yyyy-mm, account, tag or merchant)"`
	Total string  `json:"total" jsonschema_description:"Sum of the amounts of the group, as text (e.g. 1250000.00)"`
	Count int     `json:"count" jsonschema_description:"Count of transactions in the group"`
	Share float64 `json:"share" jsonschema_description:"Total of the group divided by grand_total (0 to 1)"`
}

// SummaryPeriod represents the period of the spending summary
type SummaryPeriod struct {
	Start string `json:"start" jsonschema_description:"Start of the period in RFC 3339 format"`
	End   string `json:"end" jsonschema_description:"End of the period in RFC 3339 format"`
}

// Summary represents the spending (or income) summary of a period
type Summary struct {
	Rows                       []*SummaryRow  `json:"rows" jsonschema_description:"Totals by group: ordered by total (descending), or by month (ascending) when group_by is month"`
	GrandTotal                 string         `json:"grand_total" jsonschema_description:"Sum of the amounts of all the transactions of the period in the currency"`
	TransactionCount           int            `json:"transaction_count" jsonschema_description:"Count of the transactions included in the grand_total"`
	Currency                   string         `json:"currency" jsonschema_description:"Currency of the summary. Transactions of accounts in other currencies are not included"`
	ExcludedOtherCurrencyCount int            `json:"excluded_other_currency_count,omitempty" jsonschema_description:"Count of transactions skipped because their account uses another currency"`
	Period                     *SummaryPeriod `json:"period" jsonschema_description:"Period of the summary"`
}

// SummaryOptions represents the parameters of the spending summary
type SummaryOptions struct {
	Start    time.Time
	End      time.Time
	Type     string
	GroupBy  string
	Currency string
	Top      int
}

type summaryGroup struct {
	total int64
	count int
}

// GetSpendingSummary aggregates the expenses (or incomes) of the period in the server, so the model does not need to read every transaction
func GetSpendingSummary(c core.Context, svc Services, user *models.User, options *SummaryOptions) (*Summary, error) {
	var dbType models.TransactionDbType

	switch options.Type {
	case TransactionTypeExpense:
		dbType = models.TRANSACTION_DB_TYPE_EXPENSE
	case TransactionTypeIncome:
		dbType = models.TRANSACTION_DB_TYPE_INCOME
	default:
		return nil, errs.ErrTransactionTypeInvalid
	}

	switch options.GroupBy {
	case GroupByCategory, GroupByMonth, GroupByAccount, GroupByTag, GroupByCommentMerchant:
	default:
		return nil, errs.ErrIncompleteOrIncorrectSubmission
	}

	if options.End.Before(options.Start) {
		return nil, errs.ErrIncompleteOrIncorrectSubmission
	}

	top := options.Top

	if top <= 0 {
		top = defaultSummaryTopGroups
	}

	currency := strings.ToUpper(strings.TrimSpace(options.Currency))

	if currency == "" {
		currency = user.DefaultCurrency
	}

	uid := user.Uid
	minTime := utils.GetMinTransactionTimeFromUnixTime(options.Start.Unix())
	maxTime := utils.GetMaxTransactionTimeFromUnixTime(options.End.Unix())

	transactions, err := svc.GetInboxItemService().GetTransactionsForSummary(c, uid, dbType, minTime, maxTime)

	if err != nil {
		return nil, err
	}

	allAccounts, err := svc.GetAccountService().GetAllAccountsByUid(c, uid)

	if err != nil {
		return nil, err
	}

	accountMap := svc.GetAccountService().GetAccountMapByList(allAccounts)
	categoryNames := map[int64]string{}
	tagNames := map[int64]string{}
	transactionTagNames := map[int64][]string{}

	if options.GroupBy == GroupByCategory {
		allCategories, err := svc.GetTransactionCategoryService().GetAllCategoriesByUid(c, uid, 0, -1)

		if err != nil {
			return nil, err
		}

		categoryNames = buildCategoryKeys(allCategories)
	}

	if options.GroupBy == GroupByTag {
		allTags, err := svc.GetTransactionTagService().GetAllTagsByUid(c, uid)

		if err != nil {
			return nil, err
		}

		for _, tag := range allTags {
			tagNames[tag.TagId] = tag.Name
		}

		tagIndexes, err := svc.GetInboxItemService().GetTagIndexesForSummary(c, uid, minTime, maxTime)

		if err != nil {
			return nil, err
		}

		for _, tagIndex := range tagIndexes {
			if name, exists := tagNames[tagIndex.TagId]; exists {
				transactionTagNames[tagIndex.TransactionId] = append(transactionTagNames[tagIndex.TransactionId], name)
			}
		}
	}

	groups := make(map[string]*summaryGroup)
	grandTotal := int64(0)
	transactionCount := 0
	excluded := 0

	addToGroup := func(key string, amount int64) {
		group, exists := groups[key]

		if !exists {
			group = &summaryGroup{}
			groups[key] = group
		}

		group.total += amount
		group.count++
	}

	for _, transaction := range transactions {
		account, exists := accountMap[transaction.AccountId]

		if !exists || account.Currency != currency {
			excluded++
			continue
		}

		grandTotal += transaction.Amount
		transactionCount++

		switch options.GroupBy {
		case GroupByCategory:
			key, exists := categoryNames[transaction.CategoryId]

			if !exists {
				key = summaryUnknownKey
			}

			addToGroup(key, transaction.Amount)
		case GroupByMonth:
			transactionTimezone := time.FixedZone("Transaction Timezone", int(transaction.TimezoneUtcOffset)*60)
			addToGroup(time.Unix(utils.GetUnixTimeFromTransactionTime(transaction.TransactionTime), 0).In(transactionTimezone).Format("2006-01"), transaction.Amount)
		case GroupByAccount:
			addToGroup(account.Name, transaction.Amount)
		case GroupByTag:
			names := transactionTagNames[transaction.TransactionId]

			if len(names) < 1 {
				addToGroup(summaryNoTagKey, transaction.Amount)
			}

			for _, name := range names {
				addToGroup(name, transaction.Amount)
			}
		case GroupByCommentMerchant:
			key := NormalizeMerchant(transaction.Comment, merchantKeyMaxLength)

			if key == "" {
				key = summaryUnknownKey
			}

			addToGroup(key, transaction.Amount)
		}
	}

	sortedKeys := make([]string, 0, len(groups))

	for key := range groups {
		sortedKeys = append(sortedKeys, key)
	}

	if options.GroupBy == GroupByMonth {
		sort.Strings(sortedKeys)
	} else {
		sort.Slice(sortedKeys, func(i, j int) bool {
			groupI, groupJ := groups[sortedKeys[i]], groups[sortedKeys[j]]

			if groupI.total != groupJ.total {
				return groupI.total > groupJ.total
			}

			return sortedKeys[i] < sortedKeys[j]
		})
	}

	rows := make([]*SummaryRow, 0, len(sortedKeys))
	otherTotal := int64(0)
	otherCount := 0

	for i, key := range sortedKeys {
		group := groups[key]

		if options.GroupBy != GroupByMonth && i >= top {
			otherTotal += group.total
			otherCount += group.count
			continue
		}

		rows = append(rows, &SummaryRow{Key: key, Total: utils.FormatAmount(group.total), Count: group.count, Share: shareOf(group.total, grandTotal)})
	}

	if otherCount > 0 {
		rows = append(rows, &SummaryRow{Key: summaryOtherKey, Total: utils.FormatAmount(otherTotal), Count: otherCount, Share: shareOf(otherTotal, grandTotal)})
	}

	return &Summary{
		Rows:                       rows,
		GrandTotal:                 utils.FormatAmount(grandTotal),
		TransactionCount:           transactionCount,
		Currency:                   currency,
		ExcludedOtherCurrencyCount: excluded,
		Period: &SummaryPeriod{
			Start: options.Start.Format(time.RFC3339),
			End:   options.End.Format(time.RFC3339),
		},
	}, nil
}

func shareOf(total int64, grandTotal int64) float64 {
	if grandTotal == 0 {
		return 0
	}

	return math.Round(float64(total)/float64(grandTotal)*10000) / 10000
}

// buildCategoryKeys returns the name used to group each secondary category, it includes the primary category when the name is used by more than one category
func buildCategoryKeys(categories []*models.TransactionCategory) map[int64]string {
	byId := make(map[int64]*models.TransactionCategory, len(categories))
	nameCount := make(map[string]int)

	for _, category := range categories {
		byId[category.CategoryId] = category

		if category.ParentCategoryId != models.LevelOneTransactionCategoryParentId {
			nameCount[category.Name]++
		}
	}

	keys := make(map[int64]string, len(categories))

	for _, category := range categories {
		if category.ParentCategoryId == models.LevelOneTransactionCategoryParentId {
			continue
		}

		key := category.Name

		if nameCount[category.Name] > 1 {
			if parent, exists := byId[category.ParentCategoryId]; exists {
				key = parent.Name + " / " + category.Name
			}
		}

		keys[category.CategoryId] = key
	}

	return keys
}
