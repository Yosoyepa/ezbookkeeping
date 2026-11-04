package aiimport

import (
	"fmt"
	"sync"
	"testing"
	"time"

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

// H1: a description which is empty (or only has the origin suffix) is not a wildcard that matches any merchant
func TestImport_EmptyCommentIsNotAWildcard(t *testing.T) {
	// case 1: the existing transaction or the row has a description and the other one does not -> not a duplicate
	env := aitest.NewEnv(t)
	seed, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("45.900", "Éxito Suba", "2026-10-15T12:00:00-05:00")}, importOptions("manual", false, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, seed.Created)

	result, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("45.900", "", "2026-10-15T14:00:00-05:00")}, importOptions("extracto", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created, "a row without description is not a duplicate of an existing transaction with description")
	assert.Equal(t, 0, result.Duplicates)

	env = aitest.NewEnv(t)
	seed, err = Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("45.900", "", "2026-10-15T12:00:00-05:00")}, importOptions("manual", false, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, seed.Created)

	result, err = Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("45.900", "Carulla", "2026-10-15T14:00:00-05:00")}, importOptions("extracto", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created, "a row with description is not a duplicate of an existing transaction without description")
	assert.Equal(t, 0, result.Duplicates)

	// case 2: both without description, same account, type and amount, the same calendar day (in the time zone of the row) -> duplicate
	result, err = Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("45.900", "", "2026-10-15T20:00:00-05:00")}, importOptions("extracto", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.Duplicates)
	assert.Equal(t, RowStatusDuplicate, result.Rows[0].Status)
	assert.NotEmpty(t, result.Rows[0].DuplicateOf)

	// case 3: both without description but in different calendar days (less than 24 hours apart) -> not a duplicate
	env = aitest.NewEnv(t)
	seed, err = Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("45.900", "", "2026-10-15T23:00:00-05:00")}, importOptions("manual", false, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, seed.Created)

	result, err = Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("45.900", "", "2026-10-16T08:00:00-05:00")}, importOptions("extracto", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created, "the next calendar day is not a duplicate when there is no description")
	assert.Equal(t, 0, result.Duplicates)
}

func TestImport_EmptyCommentIsNotAWildcardInTheSameBatch(t *testing.T) {
	// a row with description and a row without it
	env := aitest.NewEnv(t)
	result, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{
		newExpenseRow("8.000", "", "2026-10-15T08:00:00-05:00"),
		newExpenseRow("8.000", "Tinto", "2026-10-15T09:00:00-05:00"),
		newExpenseRow("8.000", "", "2026-10-15T10:00:00-05:00"), // same day as row 0, both without description -> duplicate of row 0
		newExpenseRow("8.000", "", "2026-10-16T06:00:00-05:00"), // 22 hours after row 0 but the next calendar day -> new
	}, importOptions("x", true, false))
	assert.Nil(t, err)

	assert.Equal(t, RowStatusCreated, result.Rows[0].Status)
	assert.Equal(t, RowStatusCreated, result.Rows[1].Status)
	assert.Equal(t, RowStatusDuplicate, result.Rows[2].Status)
	assert.Equal(t, "row:0", result.Rows[2].DuplicateOf)
	assert.Equal(t, RowStatusCreated, result.Rows[3].Status)
	assert.Equal(t, 3, result.Created)
	assert.Equal(t, 1, result.Duplicates)
	assert.Equal(t, int64(3), env.CountTransactions())

	// the dry run classifies the same way
	env = aitest.NewEnv(t)
	dry, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{
		newExpenseRow("8.000", "Tinto", "2026-10-15T08:00:00-05:00"),
		newExpenseRow("8.000", "", "2026-10-15T09:00:00-05:00"),
	}, importOptions("x", true, true))
	assert.Nil(t, err)
	assert.Equal(t, 2, dry.WouldCreate)
	assert.Equal(t, 0, dry.Duplicates)
}

// H5: the dry run applies the same domain validations as the write
func TestImport_DryRunRejectsWhatTheWriteRejects(t *testing.T) {
	env := aitest.NewEnv(t)

	// a balance modification in the account Efectivo on the 20th: nothing can be added to it before
	env.Insert(&models.Transaction{
		TransactionId:    9001,
		Uid:              aitest.Uid,
		Type:             models.TRANSACTION_DB_TYPE_MODIFY_BALANCE,
		TransactionTime:  utils.GetMinTransactionTimeFromUnixTime(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC).Unix()),
		AccountId:        env.Cash,
		RelatedAccountId: env.Cash,
	})

	transfer := func(destinationAccount string, amount string, destinationAmount string) *TxRow {
		return &TxRow{Type: TransactionTypeTransfer, Time: "2026-10-25T12:00:00-05:00", SecondaryCategoryName: "Transferencia", AccountName: "Efectivo", Amount: amount, DestinationAccountName: destinationAccount, DestinationAmount: destinationAmount}
	}

	rows := []*TxRow{
		transfer("Bancolombia", "1.000", "1.000"),                                  // 0: valid
		transfer("Bancolombia", "1.000", "2.000"),                                  // 1: the same currency with other amounts
		transfer("Efectivo", "1.000", "1.000"),                                     // 2: the same account as source and destination
		transfer("Dólares", "1.000", "250"),                                        // 3: other currency, the amounts can differ -> valid
		newExpenseRow("5.000", "antes del ajuste", "2026-10-10T12:00:00-05:00"),    // 4: before the balance modification of Efectivo
		{Type: TransactionTypeExpense, Time: "2026-10-10T12:00:00-05:00", SecondaryCategoryName: "Restaurantes", AccountName: "Bancolombia", Amount: "5.000", Comment: "otra cuenta"}, // 5: other account -> valid
	}
	expected := []string{RowStatusWouldCreate, RowStatusError, RowStatusError, RowStatusWouldCreate, RowStatusError, RowStatusWouldCreate}

	dry, err := Import(env.Ctx, aitest.Services{}, env.User, rows, importOptions("x", false, true))
	assert.Nil(t, err)

	for i, status := range expected {
		assert.Equal(t, status, dry.Rows[i].Status, "dry run row %d", i)
	}

	assert.Contains(t, dry.Rows[1].Message, "amount not equal")
	assert.Equal(t, 3, dry.WouldCreate)
	assert.Equal(t, 3, dry.Errors)
	assert.Equal(t, int64(1), env.CountTransactions()) // only the balance modification

	// the real run gives the same classification
	real, err := Import(env.Ctx, aitest.Services{}, env.User, rows, importOptions("x", false, false))
	assert.Nil(t, err)

	for i, status := range expected {
		if status == RowStatusWouldCreate {
			status = RowStatusCreated
		}

		assert.Equal(t, status, real.Rows[i].Status, "real run row %d", i)
	}

	assert.Equal(t, dry.Errors, real.Errors)
	assert.Equal(t, dry.WouldCreate, real.Created)
	assert.Equal(t, dry.Rows[1].Message, real.Rows[1].Message)
}

// H2: simultaneous imports of the same statement create each transaction once
func TestImport_ConcurrentImportsOfTheSameRowsCreateThemOnce(t *testing.T) {
	env := aitest.NewEnv(t)

	const rowCount = 20
	const callers = 6

	// every row has its own amount and day, so no row is a duplicate of another row of the same statement
	newRows := func() []*TxRow {
		rows := make([]*TxRow, rowCount)

		for i := range rows {
			rows[i] = newExpenseRow(fmt.Sprintf("%d", 1000+i), "Compra", fmt.Sprintf("2026-10-%02dT12:00:00-05:00", i+1))
		}

		return rows
	}

	results := make([]*Result, callers)
	errors := make([]error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < callers; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errors[i] = Import(env.Ctx, aitest.Services{}, env.User, newRows(), importOptions("bancolombia-2026-10", true, false))
		}(i)
	}

	close(start)
	wg.Wait()

	created, duplicates := 0, 0

	for i := 0; i < callers; i++ {
		assert.Nil(t, errors[i])
		created += results[i].Created
		duplicates += results[i].Duplicates
	}

	assert.Equal(t, rowCount, created)
	assert.Equal(t, rowCount*(callers-1), duplicates)
	assert.Equal(t, int64(rowCount), env.CountTransactions())
	assert.Equal(t, 1, countTagsNamed(t, env, AITagName))
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
