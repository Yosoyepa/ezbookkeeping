package aiimport

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport/aitest"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

func seedSummaryData(t *testing.T, env *aitest.Env) {
	t.Helper()

	rows := []*TxRow{
		{Type: "expense", Time: "2026-10-05T12:00:00-05:00", SecondaryCategoryName: "Restaurantes", AccountName: "Efectivo", Amount: "100.000", Comment: "Exito Suba", Tags: []string{"comida"}},
		{Type: "expense", Time: "2026-10-10T12:00:00-05:00", SecondaryCategoryName: "Mercado", AccountName: "Bancolombia", Amount: "300.000", Comment: "EXITO SUBA"},
		{Type: "expense", Time: "2026-10-12T12:00:00-05:00", SecondaryCategoryName: "Taxi", AccountName: "Efectivo", Amount: "50.000", Comment: "Uber"},
		{Type: "expense", Time: "2026-10-31T23:30:00-05:00", SecondaryCategoryName: "Taxi", AccountName: "Efectivo", Amount: "20.000", Comment: "Uber"}, // November 1st in UTC
		{Type: "expense", Time: "2026-11-03T12:00:00-05:00", SecondaryCategoryName: "Restaurantes", AccountName: "Efectivo", Amount: "200.000", Comment: "Éxito"},
		{Type: "income", Time: "2026-10-01T09:00:00-05:00", SecondaryCategoryName: "Salario", AccountName: "Bancolombia", Amount: "3.500.000", Comment: "Nómina"},
		{Type: "expense", Time: "2026-10-15T12:00:00-05:00", SecondaryCategoryName: "Restaurantes", AccountName: "Dólares", Amount: "10", Comment: "Starbucks"},
	}

	result, err := Import(env.Ctx, aitest.Services{}, env.User, rows, importOptions("seed", false, false))
	assert.Nil(t, err)
	assert.Equal(t, len(rows), result.Created)

	// transactions without comment and tags, in the two categories named "Otros"
	env.Insert(
		&models.Transaction{TransactionId: 9001, Uid: aitest.Uid, Type: models.TRANSACTION_DB_TYPE_EXPENSE, CategoryId: 113, AccountId: env.Cash,
			TransactionTime: utils.GetMinTransactionTimeFromUnixTime(time.Date(2026, 11, 4, 17, 0, 0, 0, time.UTC).Unix()), Amount: 1000000},
		&models.Transaction{TransactionId: 9002, Uid: aitest.Uid, Type: models.TRANSACTION_DB_TYPE_EXPENSE, CategoryId: 112, AccountId: env.Cash,
			TransactionTime: utils.GetMinTransactionTimeFromUnixTime(time.Date(2026, 11, 5, 17, 0, 0, 0, time.UTC).Unix()), Amount: 500000},
	)
}

func newSummaryOptions(groupBy string) *SummaryOptions {
	return &SummaryOptions{
		Start:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.FixedZone("COT", -5*3600)),
		End:     time.Date(2026, 11, 30, 23, 59, 59, 0, time.FixedZone("COT", -5*3600)),
		Type:    "expense",
		GroupBy: groupBy,
	}
}

type summaryExpectation struct {
	key   string
	total string
	count int
}

func assertSummaryRows(t *testing.T, summary *Summary, expected []summaryExpectation) {
	t.Helper()
	actual := make([]summaryExpectation, 0, len(summary.Rows))

	for _, row := range summary.Rows {
		actual = append(actual, summaryExpectation{row.Key, row.Total, row.Count})
	}

	assert.Equal(t, expected, actual)
}

func TestGetSpendingSummary_ByCategory(t *testing.T) {
	env := aitest.NewEnv(t)
	seedSummaryData(t, env)

	summary, err := GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, newSummaryOptions(GroupByCategory))
	assert.Nil(t, err)

	assertSummaryRows(t, summary, []summaryExpectation{
		{"Mercado", "300000.00", 1},
		{"Restaurantes", "300000.00", 2},
		{"Taxi", "70000.00", 2},
		{"Alimentación / Otros", "10000.00", 1},
		{"Transporte / Otros", "5000.00", 1},
	})
	assert.Equal(t, "685000.00", summary.GrandTotal)
	assert.Equal(t, 7, summary.TransactionCount)
	assert.Equal(t, "COP", summary.Currency)
	assert.Equal(t, 1, summary.ExcludedOtherCurrencyCount) // the expense of the USD account
	assert.Equal(t, 0.4380, summary.Rows[0].Share)
	assert.Equal(t, "2026-10-01T00:00:00-05:00", summary.Period.Start)
}

func TestGetSpendingSummary_TopGroupsAreAddedUpInOther(t *testing.T) {
	env := aitest.NewEnv(t)
	seedSummaryData(t, env)

	options := newSummaryOptions(GroupByCategory)
	options.Top = 2

	summary, err := GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, options)
	assert.Nil(t, err)

	assertSummaryRows(t, summary, []summaryExpectation{
		{"Mercado", "300000.00", 1},
		{"Restaurantes", "300000.00", 2},
		{"(other)", "85000.00", 4},
	})
	assert.Equal(t, "685000.00", summary.GrandTotal)
}

func TestGetSpendingSummary_ByMonthUsesTheTimezoneOfTheTransaction(t *testing.T) {
	env := aitest.NewEnv(t)
	seedSummaryData(t, env)

	summary, err := GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, newSummaryOptions(GroupByMonth))
	assert.Nil(t, err)

	assertSummaryRows(t, summary, []summaryExpectation{
		{"2026-10", "470000.00", 4},
		{"2026-11", "215000.00", 3},
	})
}

func TestGetSpendingSummary_ByAccount(t *testing.T) {
	env := aitest.NewEnv(t)
	seedSummaryData(t, env)

	summary, err := GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, newSummaryOptions(GroupByAccount))
	assert.Nil(t, err)

	assertSummaryRows(t, summary, []summaryExpectation{
		{"Efectivo", "385000.00", 6},
		{"Bancolombia", "300000.00", 1},
	})
}

func TestGetSpendingSummary_ByTag(t *testing.T) {
	env := aitest.NewEnv(t)
	seedSummaryData(t, env)

	summary, err := GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, newSummaryOptions(GroupByTag))
	assert.Nil(t, err)

	assertSummaryRows(t, summary, []summaryExpectation{
		{AITagName, "670000.00", 5},
		{"comida", "100000.00", 1},
		{"(no tag)", "15000.00", 2},
	})
}

func TestGetSpendingSummary_ByCommentMerchant(t *testing.T) {
	env := aitest.NewEnv(t)
	seedSummaryData(t, env)

	summary, err := GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, newSummaryOptions(GroupByCommentMerchant))
	assert.Nil(t, err)

	assertSummaryRows(t, summary, []summaryExpectation{
		{"exito suba", "400000.00", 2},
		{"exito", "200000.00", 1},
		{"uber", "70000.00", 2},
		{"(unknown)", "15000.00", 2},
	})
}

func TestGetSpendingSummary_IncomeCurrencyAndPeriod(t *testing.T) {
	env := aitest.NewEnv(t)
	seedSummaryData(t, env)

	options := newSummaryOptions(GroupByCategory)
	options.Type = "income"
	summary, err := GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, options)
	assert.Nil(t, err)
	assertSummaryRows(t, summary, []summaryExpectation{{"Salario", "3500000.00", 1}})
	assert.Equal(t, "3500000.00", summary.GrandTotal)
	assert.Equal(t, 1.0, summary.Rows[0].Share)

	options = newSummaryOptions(GroupByCategory)
	options.Currency = "usd"
	summary, err = GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, options)
	assert.Nil(t, err)
	assertSummaryRows(t, summary, []summaryExpectation{{"Restaurantes", "10.00", 1}})
	assert.Equal(t, "USD", summary.Currency)
	assert.Equal(t, 7, summary.ExcludedOtherCurrencyCount)

	options = newSummaryOptions(GroupByCategory)
	options.Start = time.Date(2026, 11, 1, 0, 0, 0, 0, time.FixedZone("COT", -5*3600))
	summary, err = GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, options)
	assert.Nil(t, err)
	assert.Equal(t, "215000.00", summary.GrandTotal)
	assert.Equal(t, 3, summary.TransactionCount)

	options = newSummaryOptions(GroupByCategory)
	options.Start = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	options.End = time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)
	summary, err = GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, options)
	assert.Nil(t, err)
	assert.Empty(t, summary.Rows)
	assert.Equal(t, "0.00", summary.GrandTotal)
}

func TestGetSpendingSummary_InvalidOptions(t *testing.T) {
	env := aitest.NewEnv(t)

	options := newSummaryOptions(GroupByCategory)
	options.Type = "transfer"
	_, err := GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, options)
	assert.Equal(t, errs.ErrTransactionTypeInvalid, err)

	_, err = GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, newSummaryOptions("weekday"))
	assert.Equal(t, errs.ErrIncompleteOrIncorrectSubmission, err)

	options = newSummaryOptions(GroupByCategory)
	options.End = options.Start.Add(-time.Hour)
	_, err = GetSpendingSummary(env.Ctx, aitest.Services{}, env.User, options)
	assert.Equal(t, errs.ErrIncompleteOrIncorrectSubmission, err)
}
