package aiimport

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport/aitest"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/services"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

func newExpenseRow(amount string, comment string, time string) *TxRow {
	return &TxRow{
		Type:                  TransactionTypeExpense,
		Time:                  time,
		SecondaryCategoryName: "Restaurantes",
		AccountName:           "Efectivo",
		Amount:                amount,
		Comment:               comment,
	}
}

func importOptions(source string, skipDuplicates bool, dryRun bool) *Options {
	return &Options{SourceLabel: source, SkipDuplicates: skipDuplicates, DryRun: dryRun, ClientIP: "127.0.0.1"}
}

func getTransaction(t *testing.T, env *aitest.Env, id string) *models.Transaction {
	t.Helper()
	transactionId, err := utils.StringToInt64(id)
	assert.Nil(t, err)

	transaction, err := services.Transactions.GetTransactionByTransactionId(env.Ctx, aitest.Uid, transactionId)
	assert.Nil(t, err)

	return transaction
}

func getTagNames(t *testing.T, env *aitest.Env, id string) []string {
	t.Helper()
	transactionId, _ := utils.StringToInt64(id)
	tagIds, err := services.TransactionTags.GetAllTagIdsOfTransactions(env.Ctx, aitest.Uid, []int64{transactionId})
	assert.Nil(t, err)

	names := make([]string, 0)

	if len(tagIds[transactionId]) < 1 {
		return names
	}

	tags, err := services.TransactionTags.GetTagsByTagIds(env.Ctx, aitest.Uid, tagIds[transactionId])
	assert.Nil(t, err)

	for _, tagId := range tagIds[transactionId] {
		names = append(names, tags[tagId].Name)
	}

	return names
}

func countTagsNamed(t *testing.T, env *aitest.Env, name string) int {
	t.Helper()
	tags, err := services.TransactionTags.GetAllTagsByUid(env.Ctx, aitest.Uid)
	assert.Nil(t, err)

	count := 0

	for _, tag := range tags {
		if tag.Name == name {
			count++
		}
	}

	return count
}

func TestImport_CreatesTransactionsWithAITagAndOrigin(t *testing.T) {
	env := aitest.NewEnv(t)

	rows := []*TxRow{
		newExpenseRow("$ 45.900", "Éxito Suba", "2026-10-15T12:00:00-05:00"),
		{Type: TransactionTypeIncome, Time: "2026-10-01T09:00:00-05:00", SecondaryCategoryName: "Salario", AccountName: "Bancolombia", Amount: "3.500.000", Tags: []string{"viaje"}},
	}

	result, err := Import(env.Ctx, aitest.Services{}, env.User, rows, importOptions("bancolombia-2026-10", true, false))
	assert.Nil(t, err)

	assert.Equal(t, 2, result.Created)
	assert.Equal(t, 0, result.Duplicates)
	assert.Equal(t, 0, result.Errors)
	assert.Equal(t, "45900.00", result.Rows[0].NormalizedAmount)
	assert.Equal(t, "3500000.00", result.Rows[1].NormalizedAmount)
	assert.Equal(t, RowStatusCreated, result.Rows[0].Status)
	assert.Equal(t, int64(2), env.CountTransactions())

	assert.Equal(t, int64(-4590000), env.AccountBalance(env.Cash))
	assert.Equal(t, int64(350000000), env.AccountBalance(env.Bank))

	expense := getTransaction(t, env, result.Rows[0].TransactionId)
	assert.Equal(t, "Éxito Suba [origen: bancolombia-2026-10]", expense.Comment)
	assert.Equal(t, int64(4590000), expense.Amount)
	assert.Equal(t, models.TRANSACTION_DB_TYPE_EXPENSE, expense.Type)
	assert.Equal(t, int16(-300), expense.TimezoneUtcOffset)
	assert.Equal(t, []string{AITagName}, getTagNames(t, env, result.Rows[0].TransactionId))

	income := getTransaction(t, env, result.Rows[1].TransactionId)
	assert.Equal(t, "[origen: bancolombia-2026-10]", income.Comment)
	assert.ElementsMatch(t, []string{"viaje", AITagName}, getTagNames(t, env, result.Rows[1].TransactionId))

	assert.Equal(t, 1, countTagsNamed(t, env, AITagName))
	assert.Equal(t, []int64{expense.TransactionId, income.TransactionId}, result.CreatedTransactionIds)
}

func TestImport_ReimportCreatesNothing(t *testing.T) {
	env := aitest.NewEnv(t)

	newRows := func() []*TxRow {
		return []*TxRow{
			newExpenseRow("45.900", "Éxito Suba", "2026-10-15T12:00:00-05:00"),
			newExpenseRow("12.500", "Tinto Juan Valdez", "2026-10-16T08:30:00-05:00"),
			{Type: TransactionTypeIncome, Time: "2026-10-01T09:00:00-05:00", SecondaryCategoryName: "Salario", AccountName: "Bancolombia", Amount: "3.500.000", Comment: "Nómina"},
		}
	}

	first, err := Import(env.Ctx, aitest.Services{}, env.User, newRows(), importOptions("bancolombia-2026-10", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 3, first.Created)

	second, err := Import(env.Ctx, aitest.Services{}, env.User, newRows(), importOptions("bancolombia-2026-10", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 0, second.Created)
	assert.Equal(t, 3, second.Duplicates)
	assert.Equal(t, int64(3), env.CountTransactions())

	for i, row := range second.Rows {
		assert.Equal(t, RowStatusDuplicate, row.Status)
		assert.Equal(t, first.Rows[i].TransactionId, row.DuplicateOf)
		assert.NotEmpty(t, row.Message)
	}

	// balances are not changed by the second import
	assert.Equal(t, int64(-(4590000 + 1250000)), env.AccountBalance(env.Cash))

	// a statement imported with another source label is still detected as duplicated
	third, err := Import(env.Ctx, aitest.Services{}, env.User, newRows(), importOptions("otro-extracto", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 0, third.Created)
	assert.Equal(t, int64(3), env.CountTransactions())
}

func TestImport_DuplicateRowsInTheSameBatch(t *testing.T) {
	env := aitest.NewEnv(t)

	rows := []*TxRow{
		newExpenseRow("8.000", "Tinto", "2026-10-15T08:00:00-05:00"),
		newExpenseRow("8.000", "TINTO", "2026-10-15T15:00:00-05:00"),
		newExpenseRow("8.000", "Tinto", "2026-10-20T15:00:00-05:00"), // 5 days later: not a duplicate
	}

	result, err := Import(env.Ctx, aitest.Services{}, env.User, rows, importOptions("x", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 2, result.Created)
	assert.Equal(t, 1, result.Duplicates)
	assert.Equal(t, RowStatusDuplicate, result.Rows[1].Status)
	assert.Equal(t, "row:0", result.Rows[1].DuplicateOf)
	assert.Equal(t, RowStatusCreated, result.Rows[2].Status)

	// with skip_duplicates=false every row is created
	env2 := aitest.NewEnv(t)
	result, err = Import(env2.Ctx, aitest.Services{}, env2.User, rows[:2], importOptions("x", false, false))
	assert.Nil(t, err)
	assert.Equal(t, 2, result.Created)
	assert.Equal(t, 0, result.Duplicates)
}

func TestImport_DuplicateOfExistingTransaction(t *testing.T) {
	env := aitest.NewEnv(t)

	manual, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("45.900", "Exito", "2026-10-15T12:00:00-05:00")}, importOptions("manual", false, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, manual.Created)

	rows := []*TxRow{
		newExpenseRow("45.900", "COMPRA EXITO SUBA", "2026-10-16T08:00:00-05:00"), // same amount, next day, similar comment: duplicate
		newExpenseRow("45.900", "COMPRA EXITO SUBA", "2026-10-18T08:00:00-05:00"), // 3 days later: new
		newExpenseRow("45.901", "COMPRA EXITO SUBA", "2026-10-15T13:00:00-05:00"), // other amount: new
		newExpenseRow("45.900", "PAGO CARULLA", "2026-10-15T13:00:00-05:00"),      // other comment: new
		{Type: TransactionTypeExpense, Time: "2026-10-15T13:00:00-05:00", SecondaryCategoryName: "Restaurantes", AccountName: "Bancolombia", Amount: "45.900", Comment: "COMPRA EXITO SUBA"}, // other account: new
		{Type: TransactionTypeIncome, Time: "2026-10-15T13:00:00-05:00", SecondaryCategoryName: "Salario", AccountName: "Efectivo", Amount: "45.900", Comment: "COMPRA EXITO SUBA"},          // other type: new
	}

	result, err := Import(env.Ctx, aitest.Services{}, env.User, rows, importOptions("extracto", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 5, result.Created)
	assert.Equal(t, 1, result.Duplicates)
	assert.Equal(t, RowStatusDuplicate, result.Rows[0].Status)
	assert.Equal(t, manual.Rows[0].TransactionId, result.Rows[0].DuplicateOf)

	for _, row := range result.Rows[1:] {
		assert.Equal(t, RowStatusCreated, row.Status)
	}
}

func TestImport_DryRunWritesNothing(t *testing.T) {
	env := aitest.NewEnv(t)

	rows := []*TxRow{
		newExpenseRow("45.900", "Éxito", "2026-10-15T12:00:00-05:00"),
		newExpenseRow("45.900", "Éxito", "2026-10-15T14:00:00-05:00"),
		newExpenseRow("1.2.3", "malo", "2026-10-15T14:00:00-05:00"),
	}

	result, err := Import(env.Ctx, aitest.Services{}, env.User, rows, importOptions("x", true, true))
	assert.Nil(t, err)

	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.WouldCreate)
	assert.Equal(t, 1, result.Duplicates)
	assert.Equal(t, 1, result.Errors)
	assert.Equal(t, RowStatusWouldCreate, result.Rows[0].Status)
	assert.Equal(t, RowStatusDuplicate, result.Rows[1].Status)
	assert.Equal(t, RowStatusError, result.Rows[2].Status)
	assert.Equal(t, "45900.00", result.Rows[0].NormalizedAmount)
	assert.Empty(t, result.Rows[0].TransactionId)
	assert.Empty(t, result.CreatedTransactionIds)

	assert.Equal(t, int64(0), env.CountTransactions())
	assert.Equal(t, int64(0), env.AccountBalance(env.Cash))
	assert.Equal(t, 0, countTagsNamed(t, env, AITagName))

	// the real run gives the same classification
	real, err := Import(env.Ctx, aitest.Services{}, env.User, rows, importOptions("x", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, real.Created)
	assert.Equal(t, 1, real.Duplicates)
	assert.Equal(t, 1, real.Errors)
}

func TestImport_RowErrors(t *testing.T) {
	env := aitest.NewEnv(t)

	manyTags := make([]string, 0)

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("tag%d", i)
		env.Insert(&models.TransactionTag{TagId: int64(600 + i), Uid: aitest.Uid, Name: name, DisplayOrder: int32(10 + i)})
		manyTags = append(manyTags, name)
	}

	valid := newExpenseRow("10.000", "valida", "2026-10-15T12:00:00-05:00")

	testCases := []struct {
		name    string
		row     *TxRow
		message string
	}{
		{"nil row", nil, "empty"},
		{"unknown type", &TxRow{Type: "gasto", Time: valid.Time, SecondaryCategoryName: "Restaurantes", AccountName: "Efectivo", Amount: "1.000"}, "type"},
		{"bad time", &TxRow{Type: "expense", Time: "15/10/2026", SecondaryCategoryName: "Restaurantes", AccountName: "Efectivo", Amount: "1.000"}, "RFC 3339"},
		{"ambiguous amount", &TxRow{Type: "expense", Time: valid.Time, SecondaryCategoryName: "Restaurantes", AccountName: "Efectivo", Amount: "1.2.3"}, "without thousands separators"},
		{"negative amount", &TxRow{Type: "expense", Time: valid.Time, SecondaryCategoryName: "Restaurantes", AccountName: "Efectivo", Amount: "-5.000"}, "greater than zero"},
		{"zero amount", &TxRow{Type: "expense", Time: valid.Time, SecondaryCategoryName: "Restaurantes", AccountName: "Efectivo", Amount: "0"}, "greater than zero"},
		{"huge amount", &TxRow{Type: "expense", Time: valid.Time, SecondaryCategoryName: "Restaurantes", AccountName: "Efectivo", Amount: "99999999999999,99"}, "too large"},
		{"unknown account", &TxRow{Type: "expense", Time: valid.Time, SecondaryCategoryName: "Restaurantes", AccountName: "Nequi", Amount: "1.000"}, "account \"Nequi\""},
		{"unknown category", &TxRow{Type: "expense", Time: valid.Time, SecondaryCategoryName: "Videojuegos", AccountName: "Efectivo", Amount: "1.000"}, "category \"Videojuegos\""},
		{"category of other type", &TxRow{Type: "expense", Time: valid.Time, SecondaryCategoryName: "Salario", AccountName: "Efectivo", Amount: "1.000"}, "category \"Salario\""},
		{"primary category", &TxRow{Type: "expense", Time: valid.Time, SecondaryCategoryName: "Alimentación", AccountName: "Efectivo", Amount: "1.000"}, "category \"Alimentación\""},
		{"unknown tag", &TxRow{Type: "expense", Time: valid.Time, SecondaryCategoryName: "Restaurantes", AccountName: "Efectivo", Amount: "1.000", Tags: []string{"nope"}}, "tag \"nope\""},
		{"too many tags", &TxRow{Type: "expense", Time: valid.Time, SecondaryCategoryName: "Restaurantes", AccountName: "Efectivo", Amount: "1.000", Tags: manyTags}, "too many tags"},
		{"transfer without destination", &TxRow{Type: "transfer", Time: valid.Time, SecondaryCategoryName: "Transferencia", AccountName: "Efectivo", Amount: "1.000"}, "destination account"},
		{"transfer without destination amount", &TxRow{Type: "transfer", Time: valid.Time, SecondaryCategoryName: "Transferencia", AccountName: "Efectivo", Amount: "1.000", DestinationAccountName: "Bancolombia"}, "destination amount"},
		{"transfer to the same currency with other amount", &TxRow{Type: "transfer", Time: valid.Time, SecondaryCategoryName: "Transferencia", AccountName: "Efectivo", Amount: "1.000", DestinationAccountName: "Bancolombia", DestinationAmount: "2.000"}, "failed to create"},
	}

	rows := []*TxRow{valid}

	for _, testCase := range testCases {
		rows = append(rows, testCase.row)
	}

	result, err := Import(env.Ctx, aitest.Services{}, env.User, rows, importOptions("x", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, len(testCases), result.Errors)
	assert.Equal(t, RowStatusCreated, result.Rows[0].Status)
	assert.Equal(t, int64(1), env.CountTransactions())

	for i, testCase := range testCases {
		row := result.Rows[i+1]
		assert.Equal(t, RowStatusError, row.Status, testCase.name)
		assert.Contains(t, row.Message, testCase.message, testCase.name)
		assert.Equal(t, i+1, row.Index)
	}

	assert.Contains(t, FormatRowErrors(result, 2), "row 1:")
	assert.Contains(t, FormatRowErrors(result, 2), "...")
}

func TestImport_Transfer(t *testing.T) {
	env := aitest.NewEnv(t)

	rows := []*TxRow{
		{Type: TransactionTypeTransfer, Time: "2026-10-15T12:00:00-05:00", SecondaryCategoryName: "Transferencia", AccountName: "Efectivo", Amount: "100.000", DestinationAccountName: "Bancolombia", DestinationAmount: "100.000", Comment: "Consignación"},
	}

	result, err := Import(env.Ctx, aitest.Services{}, env.User, rows, importOptions("x", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, int64(-10000000), env.AccountBalance(env.Cash))
	assert.Equal(t, int64(10000000), env.AccountBalance(env.Bank))

	// importing it again is a duplicate
	result, err = Import(env.Ctx, aitest.Services{}, env.User, rows, importOptions("x", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.Duplicates)
}

func TestImport_ReusesExistingAITag(t *testing.T) {
	env := aitest.NewEnv(t)
	env.Insert(&models.TransactionTag{TagId: 700, Uid: aitest.Uid, Name: AITagName, DisplayOrder: 9})

	result, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("1.000", "a", "2026-10-15T12:00:00-05:00")}, importOptions("x", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, countTagsNamed(t, env, AITagName))
	assert.Equal(t, []string{AITagName}, getTagNames(t, env, result.Rows[0].TransactionId))
}

func TestImport_UserCannotEditTransactions(t *testing.T) {
	env := aitest.NewEnv(t)
	env.User.TransactionEditScope = models.TRANSACTION_EDIT_SCOPE_NONE

	result, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("1.000", "a", "2026-10-15T12:00:00-05:00")}, importOptions("x", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.Errors)
	assert.Contains(t, result.Rows[0].Message, "not editable")
}

func TestImport_LongCommentIsShortened(t *testing.T) {
	env := aitest.NewEnv(t)

	long := ""

	for i := 0; i < 40; i++ {
		long += "abcdefghij"
	}

	result, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("1.000", long, "2026-10-15T12:00:00-05:00")}, importOptions("inbox:1", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created)

	transaction := getTransaction(t, env, result.Rows[0].TransactionId)
	assert.Equal(t, 255, len([]rune(transaction.Comment)))
	assert.Contains(t, transaction.Comment, "[origen: inbox:1]")
}
