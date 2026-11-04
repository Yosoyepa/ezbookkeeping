package aiimport

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/aiimport/aitest"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

func strPtr(s string) *string { return &s }

func importOneExpense(t *testing.T, env *aitest.Env) (string, int64) {
	t.Helper()
	result, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{newExpenseRow("45.900", "Éxito", "2026-10-15T12:00:00-05:00")}, importOptions("bancolombia-2026-10", true, false))
	assert.Nil(t, err)
	assert.Equal(t, 1, result.Created)

	id, _ := utils.StringToInt64(result.Rows[0].TransactionId)

	return result.Rows[0].TransactionId, id
}

func TestUpdateTransaction_RequiresAChange(t *testing.T) {
	env := aitest.NewEnv(t)
	_, id := importOneExpense(t, env)

	_, err := UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &UpdateOptions{TransactionId: id})
	assert.Equal(t, errs.ErrIncompleteOrIncorrectSubmission, err)

	_, err = UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &UpdateOptions{TransactionId: 424242, Comment: strPtr("x")})
	assert.Equal(t, errs.ErrTransactionNotFound, err)
}

func TestUpdateTransaction_DryRunDoesNotWrite(t *testing.T) {
	env := aitest.NewEnv(t)
	idText, id := importOneExpense(t, env)

	result, err := UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &UpdateOptions{TransactionId: id, CategoryName: strPtr("Mercado"), Amount: strPtr("50.000"), DryRun: true})
	assert.Nil(t, err)
	assert.True(t, result.Changed)
	assert.Equal(t, "Restaurantes", result.Before.CategoryName)
	assert.Equal(t, "Mercado", result.After.CategoryName)
	assert.Equal(t, "45900.00", result.Before.Amount)
	assert.Equal(t, "50000.00", result.After.Amount)
	assert.Equal(t, idText, result.After.Id)

	transaction := getTransaction(t, env, idText)
	assert.Equal(t, int64(101), transaction.CategoryId)
	assert.Equal(t, int64(4590000), transaction.Amount)
	assert.Equal(t, int64(-4590000), env.AccountBalance(env.Cash))
}

func TestUpdateTransaction_CategoryTagsAndCommentKeepTheOrigin(t *testing.T) {
	env := aitest.NewEnv(t)
	idText, id := importOneExpense(t, env)

	tags := []string{"viaje", "comida"}
	result, err := UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &UpdateOptions{TransactionId: id, CategoryName: strPtr("Mercado"), Tags: &tags, Comment: strPtr("Mercado Éxito")})
	assert.Nil(t, err)
	assert.True(t, result.Changed)
	assert.Equal(t, []string{AITagName}, result.Before.Tags)
	assert.Equal(t, []string{"viaje", "comida"}, result.After.Tags)
	assert.Equal(t, "Mercado Éxito [origen: bancolombia-2026-10]", result.After.Comment)

	transaction := getTransaction(t, env, idText)
	assert.Equal(t, int64(102), transaction.CategoryId)
	assert.Equal(t, "Mercado Éxito [origen: bancolombia-2026-10]", transaction.Comment)
	assert.ElementsMatch(t, []string{"viaje", "comida"}, getTagNames(t, env, idText)) // the tags were replaced

	// a comment with its own origin is saved as it is
	result, err = UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &UpdateOptions{TransactionId: id, Comment: strPtr("Otro [origen: manual]")})
	assert.Nil(t, err)
	assert.Equal(t, "Otro [origen: manual]", result.After.Comment)

	// an empty list removes all the tags
	noTags := []string{}
	result, err = UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &UpdateOptions{TransactionId: id, Tags: &noTags})
	assert.Nil(t, err)
	assert.Empty(t, result.After.Tags)
	assert.Empty(t, getTagNames(t, env, idText))
}

func TestUpdateTransaction_AmountTimeAndAccount(t *testing.T) {
	env := aitest.NewEnv(t)
	idText, id := importOneExpense(t, env)

	result, err := UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &UpdateOptions{TransactionId: id, Amount: strPtr("$ 50.000")})
	assert.Nil(t, err)
	assert.Equal(t, "50000.00", result.After.Amount)
	assert.Equal(t, int64(-5000000), env.AccountBalance(env.Cash))

	result, err = UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &UpdateOptions{TransactionId: id, Time: strPtr("2026-10-20T10:00:00-05:00")})
	assert.Nil(t, err)
	assert.Equal(t, "2026-10-20T10:00:00-05:00", result.After.Time)
	assert.Equal(t, "2026-10-20T10:00:00-05:00", getTransactionInfoTime(t, env, id))

	result, err = UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &UpdateOptions{TransactionId: id, AccountName: strPtr("Bancolombia")})
	assert.Nil(t, err)
	assert.Equal(t, "Bancolombia", result.After.AccountName)
	assert.Equal(t, int64(0), env.AccountBalance(env.Cash))
	assert.Equal(t, int64(-5000000), env.AccountBalance(env.Bank))
	assert.Equal(t, env.Bank, getTransaction(t, env, idText).AccountId)
}

func getTransactionInfoTime(t *testing.T, env *aitest.Env, id int64) string {
	t.Helper()
	lookups, err := loadUpdateLookups(env.Ctx, aitest.Services{}, aitest.Uid)
	assert.Nil(t, err)

	transaction := getTransaction(t, env, utils.Int64ToString(id))

	return lookups.toInfo(transaction, nil).Time
}

func TestUpdateTransaction_InvalidChanges(t *testing.T) {
	env := aitest.NewEnv(t)
	_, id := importOneExpense(t, env)

	testCases := []struct {
		name    string
		options UpdateOptions
		message string
	}{
		{"unknown category", UpdateOptions{CategoryName: strPtr("Videojuegos")}, "category \"Videojuegos\""},
		{"income category for an expense", UpdateOptions{CategoryName: strPtr("Salario")}, "category \"Salario\""},
		{"unknown account", UpdateOptions{AccountName: strPtr("Nequi")}, "account \"Nequi\""},
		{"account with another currency", UpdateOptions{AccountName: strPtr("Dólares")}, "another currency"},
		{"ambiguous amount", UpdateOptions{Amount: strPtr("1.2.3")}, "without thousands separators"},
		{"zero amount", UpdateOptions{Amount: strPtr("0")}, "greater than zero"},
		{"negative amount", UpdateOptions{Amount: strPtr("-5.000")}, "greater than zero"},
		{"invalid time", UpdateOptions{Time: strPtr("20/10/2026")}, "RFC 3339"},
		{"unknown tag", UpdateOptions{Tags: &[]string{"nope"}}, "tag \"nope\""},
	}

	for _, testCase := range testCases {
		options := testCase.options
		options.TransactionId = id
		options.DryRun = true

		result, err := UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &options)
		assert.Nil(t, result, testCase.name)
		assert.NotNil(t, err, testCase.name)

		if err != nil {
			assert.Contains(t, err.Error(), testCase.message, testCase.name)
		}
	}
}

func TestUpdateTransaction_TransferAmountMovesBothSides(t *testing.T) {
	env := aitest.NewEnv(t)

	imported, err := Import(env.Ctx, aitest.Services{}, env.User, []*TxRow{
		{Type: TransactionTypeTransfer, Time: "2026-10-15T12:00:00-05:00", SecondaryCategoryName: "Transferencia", AccountName: "Efectivo", Amount: "100.000", DestinationAccountName: "Bancolombia", DestinationAmount: "100.000"},
	}, importOptions("x", true, false))
	assert.Nil(t, err)
	id, _ := utils.StringToInt64(imported.Rows[0].TransactionId)

	result, err := UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &UpdateOptions{TransactionId: id, Amount: strPtr("150.000")})
	assert.Nil(t, err)
	assert.Equal(t, "150000.00", result.After.Amount)
	assert.Equal(t, "150000.00", result.After.DestinationAmount)
	assert.Equal(t, "Bancolombia", result.After.DestinationAccountName)
	assert.Equal(t, int64(-15000000), env.AccountBalance(env.Cash))
	assert.Equal(t, int64(15000000), env.AccountBalance(env.Bank))
}

func TestUpdateTransaction_UserCannotEdit(t *testing.T) {
	env := aitest.NewEnv(t)
	_, id := importOneExpense(t, env)
	env.User.TransactionEditScope = 0

	_, err := UpdateTransaction(env.Ctx, aitest.Services{}, env.User, &UpdateOptions{TransactionId: id, Comment: strPtr("x"), DryRun: true})
	assert.Equal(t, errs.ErrCannotModifyTransactionWithThisTransactionTime, err)
}
