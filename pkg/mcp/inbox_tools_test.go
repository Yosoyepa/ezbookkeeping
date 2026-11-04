package mcp

import (
	"bytes"
	"encoding/base64"
	stdjson "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"net/http"
	"net/http/httptest"
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
	"github.com/mayswind/ezbookkeeping/pkg/services"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

type testToolResult struct {
	Content           []map[string]any `json:"content"`
	StructuredContent map[string]any   `json:"structuredContent"`
}

// callTool calls the tool through the container, and serializes the response like the server does
func callTool(t *testing.T, env *aitest.Env, name string, arguments any) (*testToolResult, error) {
	t.Helper()

	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	ginContext.Request.Header.Set(MCPProtocolVersionHeaderName, string(MCPProtocolVersion20250618))
	c := core.WrapWebContext(ginContext, nil)

	request := &MCPCallToolRequest{Name: name}

	if arguments != nil {
		argumentsBytes, err := stdjson.Marshal(arguments)
		assert.Nil(t, err)
		request.Arguments = jsontext.Value(argumentsBytes)
	}

	response, err := Container.HandleTool(c, request, env.User, env.Config, aitest.Services{})

	if err != nil {
		return nil, err
	}

	responseBytes, err := jsonv2.Marshal(response)
	assert.Nil(t, err)

	result := &testToolResult{}
	assert.Nil(t, stdjson.Unmarshal(responseBytes, result))

	return result, nil
}

// textOf returns the parsed json of the first (text) content block of the result
func textOf(t *testing.T, result *testToolResult) map[string]any {
	t.Helper()
	assert.GreaterOrEqual(t, len(result.Content), 1)
	assert.Equal(t, "text", result.Content[0]["type"])

	parsed := map[string]any{}
	assert.Nil(t, stdjson.Unmarshal([]byte(result.Content[0]["text"].(string)), &parsed))

	return parsed
}

func expenseArgs(amount string, comment string, time string) map[string]any {
	return map[string]any{
		"type": "expense", "time": time, "category_name": "Restaurantes", "account_name": "Efectivo", "amount": amount, "comment": comment,
	}
}

func rowsOf(result map[string]any) []any {
	return result["rows"].([]any)
}

func TestInboxToolsAreRegistered(t *testing.T) {
	initializeMCPContainerTest(t)

	tools := map[string]*MCPTool{}

	for _, tool := range Container.GetMCPTools() {
		tools[tool.Name] = tool
	}

	// the 7 tools of upstream are still there
	for _, name := range []string{"query_all_accounts", "query_all_accounts_balance", "query_all_transaction_categories", "query_all_transaction_tags", "query_latest_exchange_rates", "query_transactions", "add_transaction"} {
		assert.Contains(t, tools, name)
	}

	newTools := []string{"list_inbox_items", "get_inbox_item", "resolve_inbox_item", "discard_inbox_item", "add_transactions_batch", "update_transaction", "get_spending_summary"}

	for _, name := range newTools {
		tool, exists := tools[name]
		assert.True(t, exists, name)

		if !exists {
			continue
		}

		assert.NotEmpty(t, tool.Description, name)
		assert.NotNil(t, tool.InputSchema, name)
		assert.NotNil(t, tool.OutputSchema, name)

		schema, err := stdjson.Marshal(tool.InputSchema)
		assert.Nil(t, err)
		assert.Contains(t, string(schema), `"type":"object"`, name)
	}

	// the tools that write require dry_run and say how to use it
	for _, name := range []string{"resolve_inbox_item", "add_transactions_batch", "update_transaction"} {
		schema, _ := stdjson.Marshal(tools[name].InputSchema)
		assert.Regexp(t, `"required":\[[^\]]*"dry_run"`, string(schema), name)
		assert.Contains(t, tools[name].Description, "Call first with dry_run=true, show the result to the user and only call again with dry_run=false after explicit confirmation.", name)
	}

	// the schema of the rows is the same as add_transaction
	schema, _ := stdjson.Marshal(tools["add_transactions_batch"].InputSchema)

	for _, field := range []string{"type", "time", "category_name", "account_name", "amount", "destination_account_name", "destination_amount", "tags", "comment", "source_label", "skip_duplicates"} {
		assert.Contains(t, string(schema), `"`+field+`"`, field)
	}

	// every tool has its own bit
	assert.Equal(t, int64(1<<15-1), core.MCPAllTools)
}

func TestMCPMixedContent_Marshal(t *testing.T) {
	blocks := []*MCPMixedContent{
		NewMCPMixedTextContent(""),
		NewMCPMixedTextContent("hola"),
		NewMCPMixedImageContent("QUJD", "image/png"),
		NewMCPMixedBlobResourceContent("inbox://items/1/a.pdf", "application/pdf", "QUJD"),
	}

	for _, marshal := range []func(any) ([]byte, error){stdjson.Marshal, func(v any) ([]byte, error) { return jsonv2.Marshal(v) }} {
		data, err := marshal(blocks)
		assert.Nil(t, err)

		parsed := []map[string]any{}
		assert.Nil(t, stdjson.Unmarshal(data, &parsed))
		assert.Equal(t, map[string]any{"type": "text", "text": ""}, parsed[0])
		assert.Equal(t, map[string]any{"type": "text", "text": "hola"}, parsed[1])
		assert.Equal(t, map[string]any{"type": "image", "mimeType": "image/png", "data": "QUJD"}, parsed[2])
		assert.Equal(t, map[string]any{"type": "resource", "resource": map[string]any{"uri": "inbox://items/1/a.pdf", "blob": "QUJD", "mimeType": "application/pdf"}}, parsed[3])
	}

	_, err := stdjson.Marshal(&MCPMixedContent{Type: "audio"})
	assert.NotNil(t, err)
}

func TestListInboxItems(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)

	receipt := env.NewReceipt("recibo del Éxito")
	statement := env.NewStatement("csv", []byte("a;b\n"))
	discarded := env.NewReceipt("")
	assert.Nil(t, services.InboxItems.DiscardInboxItem(env.Ctx, aitest.Uid, discarded.ItemId, "borroso"))

	result, err := callTool(t, env, "list_inbox_items", map[string]any{})
	assert.Nil(t, err)
	parsed := textOf(t, result)
	assert.Equal(t, float64(2), parsed["total_count"])
	items := parsed["items"].([]any)
	assert.Equal(t, 2, len(items))
	assert.Equal(t, result.StructuredContent["total_count"], parsed["total_count"])

	first := items[1].(map[string]any) // the oldest
	assert.Equal(t, utils.Int64ToString(receipt.ItemId), first["id"])
	assert.Equal(t, "receipt", first["kind"])
	assert.Equal(t, "pending", first["status"])
	assert.Equal(t, "recibo del Éxito", first["note"])
	assert.Equal(t, "recibo.jpg", first["original_file_name"])
	_, err2 := time.Parse(time.RFC3339, first["created_time"].(string))
	assert.Nil(t, err2)

	result, err = callTool(t, env, "list_inbox_items", map[string]any{"kind": "statement"})
	assert.Nil(t, err)
	items = textOf(t, result)["items"].([]any)
	assert.Equal(t, 1, len(items))
	assert.Equal(t, utils.Int64ToString(statement.ItemId), items[0].(map[string]any)["id"])

	result, err = callTool(t, env, "list_inbox_items", map[string]any{"status": "discarded"})
	assert.Nil(t, err)
	items = textOf(t, result)["items"].([]any)
	assert.Equal(t, 1, len(items))
	assert.Equal(t, "discarded", items[0].(map[string]any)["status"])

	result, err = callTool(t, env, "list_inbox_items", map[string]any{"status": "all", "limit": 2})
	assert.Nil(t, err)
	parsed = textOf(t, result)
	assert.Equal(t, float64(3), parsed["total_count"])
	assert.Equal(t, 2, len(parsed["items"].([]any)))

	_, err = callTool(t, env, "list_inbox_items", map[string]any{"status": "borrado"})
	assert.Equal(t, errs.ErrInboxItemStatusInvalid, err)

	_, err = callTool(t, env, "list_inbox_items", map[string]any{"kind": "foto"})
	assert.Equal(t, errs.ErrInboxItemKindInvalid, err)
}

func TestGetInboxItem_Receipt(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)
	receipt := env.NewReceipt("nota")

	result, err := callTool(t, env, "get_inbox_item", map[string]any{"id": utils.Int64ToString(receipt.ItemId)})
	assert.Nil(t, err)
	assert.Equal(t, 2, len(result.Content))

	meta := textOf(t, result)
	assert.Equal(t, "receipt", meta["kind"])
	assert.Equal(t, "pending", meta["status"])
	assert.Equal(t, "nota", meta["note"])
	assert.Equal(t, "image/jpeg", meta["mime_type"])
	assert.Equal(t, true, meta["content_included"])
	assert.Equal(t, float64(len(aitest.FakeJpeg)), meta["size_bytes"])
	assert.Nil(t, meta["download_url"])
	assert.Equal(t, meta["id"], result.StructuredContent["id"])

	image := result.Content[1]
	assert.Equal(t, "image", image["type"])
	assert.Equal(t, "image/jpeg", image["mimeType"])
	assert.Equal(t, base64.StdEncoding.EncodeToString(aitest.FakeJpeg), image["data"])
}

func TestGetInboxItem_TextStatements(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)

	utf8Csv := "fecha;descripcion;valor\n15/10/2026;ÉXITO;-45.900\n"
	statement := env.NewStatement("csv", []byte(utf8Csv))

	result, err := callTool(t, env, "get_inbox_item", map[string]any{"id": utils.Int64ToString(statement.ItemId)})
	assert.Nil(t, err)
	assert.Equal(t, 2, len(result.Content))
	assert.Equal(t, "text", result.Content[1]["type"])
	assert.Equal(t, utf8Csv, result.Content[1]["text"])

	meta := textOf(t, result)
	assert.Equal(t, "statement", meta["kind"])
	assert.Equal(t, "text/csv", meta["mime_type"])
	assert.NotEmpty(t, meta["download_url"])

	// the file saved by many banks is in Windows-1252 and starts with a BOM
	latin1 := env.NewStatement("txt", append([]byte{0xEF, 0xBB, 0xBF}, []byte("Descripci\xf3n;A\xf1o\n")...))
	result, err = callTool(t, env, "get_inbox_item", map[string]any{"id": utils.Int64ToString(latin1.ItemId)})
	assert.Nil(t, err)
	assert.Equal(t, "Descripción;Año\n", result.Content[1]["text"])
}

func TestGetInboxItem_BinaryStatementsHaveAResourceAndADownloadUrl(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)

	pdfContent := []byte("%PDF-1.4 fake statement")
	statement := env.NewStatement("pdf", pdfContent)

	result, err := callTool(t, env, "get_inbox_item", map[string]any{"id": utils.Int64ToString(statement.ItemId)})
	assert.Nil(t, err)
	assert.Equal(t, 2, len(result.Content))

	resource := result.Content[1]
	assert.Equal(t, "resource", resource["type"])
	assert.Equal(t, map[string]any{
		"uri":      "inbox://items/" + utils.Int64ToString(statement.ItemId) + "/extracto.pdf",
		"mimeType": "application/pdf",
		"blob":     base64.StdEncoding.EncodeToString(pdfContent),
	}, resource["resource"])

	meta := textOf(t, result)
	downloadUrl := meta["download_url"].(string)
	assert.True(t, strings.HasPrefix(downloadUrl, "http://localhost:8080/inbox/download/"))

	// the url works once, for at most 15 minutes, for this item
	token := strings.TrimPrefix(downloadUrl, "http://localhost:8080/inbox/download/")
	uid, itemId, err := aiimport.ParseDownloadToken(env.Config.SecretKey, token, time.Now())
	assert.Nil(t, err)
	assert.Equal(t, aitest.Uid, uid)
	assert.Equal(t, statement.ItemId, itemId)
	_, _, err = aiimport.ParseDownloadToken(env.Config.SecretKey, token, time.Now())
	assert.NotNil(t, err)

	token = strings.TrimPrefix(textOf(t, mustCall(t, env, "get_inbox_item", map[string]any{"id": utils.Int64ToString(statement.ItemId)}))["download_url"].(string), "http://localhost:8080/inbox/download/")
	_, _, err = aiimport.ParseDownloadToken(env.Config.SecretKey, token, time.Now().Add(15*time.Minute+time.Minute))
	assert.NotNil(t, err)

	// a big file is not embedded
	big := env.NewStatement("xlsx", bytes.Repeat([]byte{1}, maxEmbeddedInboxFileSize+1))
	result, err = callTool(t, env, "get_inbox_item", map[string]any{"id": utils.Int64ToString(big.ItemId)})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(result.Content))
	meta = textOf(t, result)
	assert.Equal(t, false, meta["content_included"])
	assert.NotEmpty(t, meta["download_url"])
}

func mustCall(t *testing.T, env *aitest.Env, name string, arguments any) *testToolResult {
	t.Helper()
	result, err := callTool(t, env, name, arguments)
	assert.Nil(t, err)

	return result
}

func TestGetInboxItem_Errors(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)

	_, err := callTool(t, env, "get_inbox_item", map[string]any{"id": "abc"})
	assert.Equal(t, errs.ErrInboxItemIdInvalid, err)

	_, err = callTool(t, env, "get_inbox_item", map[string]any{"id": "123"})
	assert.Equal(t, errs.ErrInboxItemNotFound, err)

	_, err = callTool(t, env, "get_inbox_item", nil)
	assert.Equal(t, errs.ErrIncompleteOrIncorrectSubmission, err)
}

func TestResolveInboxItem_Tool(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)
	receipt := env.NewReceipt("")
	id := utils.Int64ToString(receipt.ItemId)
	rows := []any{expenseArgs("$ 45.900", "Éxito Suba", "2026-10-15T12:00:00-05:00")}

	// dry_run is required
	_, err := callTool(t, env, "resolve_inbox_item", map[string]any{"id": id, "transactions": rows})
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "dry_run is required")

	result, err := callTool(t, env, "resolve_inbox_item", map[string]any{"id": id, "transactions": rows, "dry_run": true})
	assert.Nil(t, err)
	parsed := textOf(t, result)
	assert.Equal(t, "pending", parsed["inbox_item_status"])
	assert.Equal(t, float64(1), parsed["would_create"])
	assert.Equal(t, float64(0), parsed["created"])
	assert.Equal(t, "would_create", rowsOf(parsed)[0].(map[string]any)["status"])
	assert.Equal(t, "45900.00", rowsOf(parsed)[0].(map[string]any)["normalized_amount"])
	assert.Equal(t, int64(0), env.CountTransactions())

	result, err = callTool(t, env, "resolve_inbox_item", map[string]any{"id": id, "transactions": rows, "dry_run": false, "note": "ok"})
	assert.Nil(t, err)
	parsed = textOf(t, result)
	assert.Equal(t, "processed", parsed["inbox_item_status"])
	assert.Equal(t, float64(1), parsed["created"])
	assert.Equal(t, int64(1), env.CountTransactions())
	transactionId := rowsOf(parsed)[0].(map[string]any)["transaction_id"].(string)
	assert.NotEmpty(t, transactionId)

	parsedId, _ := utils.StringToInt64(transactionId)
	pictures, err := services.TransactionPictures.GetPictureInfosByTransactionId(env.Ctx, aitest.Uid, parsedId)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(pictures))

	_, err = callTool(t, env, "resolve_inbox_item", map[string]any{"id": id, "transactions": rows, "dry_run": false})
	assert.Equal(t, errs.ErrInboxItemNotPending, err)

	// the row limits
	_, err = callTool(t, env, "resolve_inbox_item", map[string]any{"id": "123", "transactions": []any{}, "dry_run": true})
	assert.Equal(t, errs.ErrInboxItemNotFound, err)
}

func TestDiscardInboxItem_Tool(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)
	receipt := env.NewReceipt("")
	id := utils.Int64ToString(receipt.ItemId)

	_, err := callTool(t, env, "discard_inbox_item", map[string]any{"id": id, "reason": "  "})
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "reason is required")

	result, err := callTool(t, env, "discard_inbox_item", map[string]any{"id": id, "reason": "foto borrosa"})
	assert.Nil(t, err)
	assert.Equal(t, map[string]any{"success": true, "id": id, "status": "discarded"}, textOf(t, result))

	item, _ := services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, receipt.ItemId)
	assert.Equal(t, models.INBOX_ITEM_STATUS_DISCARDED, item.Status)
	assert.Equal(t, "foto borrosa", item.ProcessedNote)

	_, err = callTool(t, env, "discard_inbox_item", map[string]any{"id": id, "reason": "otra vez"})
	assert.Equal(t, errs.ErrInboxItemNotPending, err)
}

func batchArgs(rows []any, source string, dryRun bool) map[string]any {
	return map[string]any{"transactions": rows, "source_label": source, "dry_run": dryRun}
}

func TestAddTransactionsBatch_ReimportCreatesNothing(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)

	rows := []any{
		expenseArgs("45.900", "Éxito Suba", "2026-10-15T12:00:00-05:00"),
		expenseArgs("12.500", "Tinto", "2026-10-16T08:30:00-05:00"),
		map[string]any{"type": "income", "time": "2026-10-01T09:00:00-05:00", "category_name": "Salario", "account_name": "Bancolombia", "amount": "3.500.000", "comment": "Nómina"},
	}

	result, err := callTool(t, env, "add_transactions_batch", batchArgs(rows, "bancolombia-2026-10", false))
	assert.Nil(t, err)
	parsed := textOf(t, result)
	assert.Equal(t, float64(3), parsed["created"])
	assert.Equal(t, float64(0), parsed["duplicates"])
	assert.Equal(t, float64(0), parsed["errors"])
	assert.Equal(t, false, parsed["dry_run"])
	assert.Equal(t, "bancolombia-2026-10", parsed["source_label"])
	assert.Equal(t, int64(3), env.CountTransactions())
	assert.Equal(t, float64(3), result.StructuredContent["created"])

	// the same statement the second time creates 0 transactions
	result, err = callTool(t, env, "add_transactions_batch", batchArgs(rows, "bancolombia-2026-10", false))
	assert.Nil(t, err)
	parsed = textOf(t, result)
	assert.Equal(t, float64(0), parsed["created"])
	assert.Equal(t, float64(3), parsed["duplicates"])
	assert.Equal(t, int64(3), env.CountTransactions())

	for _, row := range rowsOf(parsed) {
		assert.Equal(t, "duplicate", row.(map[string]any)["status"])
		assert.NotEmpty(t, row.(map[string]any)["duplicate_of"])
	}

	// skip_duplicates=false forces it
	args := batchArgs(rows[:1], "bancolombia-2026-10", false)
	args["skip_duplicates"] = false
	result, err = callTool(t, env, "add_transactions_batch", args)
	assert.Nil(t, err)
	assert.Equal(t, float64(1), textOf(t, result)["created"])
	assert.Equal(t, int64(4), env.CountTransactions())
}

func TestAddTransactionsBatch_ProcessesTheInboxStatement(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)

	statement := env.NewStatement("csv", []byte("fecha;valor\n"))
	receipt := env.NewReceipt("")
	id := utils.Int64ToString(statement.ItemId)
	rows := []any{
		expenseArgs("45.900", "Éxito Suba", "2026-10-15T12:00:00-05:00"),
		expenseArgs("12.500", "Tinto", "2026-10-16T08:30:00-05:00"),
	}

	withItem := func(itemId string, dryRun bool) map[string]any {
		args := batchArgs(rows, "bancolombia-2026-10", dryRun)
		args["inbox_item_id"] = itemId

		return args
	}

	// a dry run validates but does not process the item
	result, err := callTool(t, env, "add_transactions_batch", withItem(id, true))
	assert.Nil(t, err)
	assert.Equal(t, "pending", textOf(t, result)["inbox_item_status"])
	item, _ := services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, statement.ItemId)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PENDING, item.Status)
	assert.Equal(t, int64(0), env.CountTransactions())

	// invalid items are rejected before writing anything
	_, err = callTool(t, env, "add_transactions_batch", withItem("abc", false))
	assert.Equal(t, errs.ErrInboxItemIdInvalid, err)
	_, err = callTool(t, env, "add_transactions_batch", withItem("123", false))
	assert.Equal(t, errs.ErrInboxItemNotFound, err)
	_, err = callTool(t, env, "add_transactions_batch", withItem(utils.Int64ToString(receipt.ItemId), false))
	assert.Contains(t, err.Error(), "must be a statement")

	env.Insert(&models.InboxItem{Uid: 999, ItemId: 777, Kind: models.INBOX_ITEM_KIND_STATEMENT, Status: models.INBOX_ITEM_STATUS_PENDING, FileExtension: "csv"})
	_, err = callTool(t, env, "add_transactions_batch", withItem("777", false))
	assert.Equal(t, errs.ErrInboxItemNotFound, err) // an item of another user
	assert.Equal(t, int64(0), env.CountTransactions())

	// the real run processes the item
	result, err = callTool(t, env, "add_transactions_batch", withItem(id, false))
	assert.Nil(t, err)
	parsed := textOf(t, result)
	assert.Equal(t, float64(2), parsed["created"])
	assert.Equal(t, "processed", parsed["inbox_item_status"])
	assert.Nil(t, parsed["warning"])

	item, _ = services.InboxItems.GetInboxItemByItemId(env.Ctx, aitest.Uid, statement.ItemId)
	assert.Equal(t, models.INBOX_ITEM_STATUS_PROCESSED, item.Status)
	assert.Equal(t, 2, len(item.GetResultTransactionIds()))
	assert.Contains(t, item.ProcessedNote, "created 2")

	// it cannot be used again
	_, err = callTool(t, env, "add_transactions_batch", withItem(id, false))
	assert.Equal(t, errs.ErrInboxItemNotPending, err)
	assert.Equal(t, int64(2), env.CountTransactions())

	// when everything is a duplicate the item stays pending
	other := env.NewStatement("csv", []byte("fecha;valor\n"))
	result, err = callTool(t, env, "add_transactions_batch", withItem(utils.Int64ToString(other.ItemId), false))
	assert.Nil(t, err)
	parsed = textOf(t, result)
	assert.Equal(t, float64(0), parsed["created"])
	assert.Equal(t, "pending", parsed["inbox_item_status"])
}

func TestAddTransactionsBatch_DryRunAndValidation(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)
	rows := []any{expenseArgs("45.900", "Éxito", "2026-10-15T12:00:00-05:00"), expenseArgs("1.2.3", "malo", "2026-10-15T12:00:00-05:00")}

	result, err := callTool(t, env, "add_transactions_batch", batchArgs(rows, "x", true))
	assert.Nil(t, err)
	parsed := textOf(t, result)
	assert.Equal(t, true, parsed["dry_run"])
	assert.Equal(t, float64(1), parsed["would_create"])
	assert.Equal(t, float64(1), parsed["errors"])
	assert.Equal(t, "error", rowsOf(parsed)[1].(map[string]any)["status"])
	assert.Contains(t, rowsOf(parsed)[1].(map[string]any)["message"], "without thousands separators")
	assert.Equal(t, int64(0), env.CountTransactions())

	_, err = callTool(t, env, "add_transactions_batch", map[string]any{"transactions": rows, "source_label": "x"})
	assert.Contains(t, err.Error(), "dry_run is required")

	_, err = callTool(t, env, "add_transactions_batch", batchArgs(rows, "  ", true))
	assert.Contains(t, err.Error(), "source_label is required")

	_, err = callTool(t, env, "add_transactions_batch", batchArgs([]any{}, "x", true))
	assert.Contains(t, err.Error(), "between 1 and 500")

	tooMany := make([]any, aiimport.MaxRowsBatch+1)

	for i := range tooMany {
		tooMany[i] = rows[0]
	}

	_, err = callTool(t, env, "add_transactions_batch", batchArgs(tooMany, "x", true))
	assert.Contains(t, err.Error(), "between 1 and 500")

	exactly := tooMany[:aiimport.MaxRowsBatch]
	result, err = callTool(t, env, "add_transactions_batch", map[string]any{"transactions": exactly, "source_label": "x", "dry_run": true, "skip_duplicates": false})
	assert.Nil(t, err)
	assert.Equal(t, float64(aiimport.MaxRowsBatch), textOf(t, result)["would_create"])
}

func TestUpdateTransaction_Tool(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)

	imported := mustCall(t, env, "add_transactions_batch", batchArgs([]any{expenseArgs("45.900", "Éxito", "2026-10-15T12:00:00-05:00")}, "extracto", false))
	id := rowsOf(textOf(t, imported))[0].(map[string]any)["transaction_id"].(string)

	result, err := callTool(t, env, "update_transaction", map[string]any{"id": id, "category_name": "Mercado", "dry_run": true})
	assert.Nil(t, err)
	parsed := textOf(t, result)
	assert.Equal(t, true, parsed["dry_run"])
	assert.Equal(t, true, parsed["changed"])
	assert.Equal(t, "Restaurantes", parsed["before"].(map[string]any)["category_name"])
	assert.Equal(t, "Mercado", parsed["after"].(map[string]any)["category_name"])

	transactionId, _ := utils.StringToInt64(id)
	transaction, _ := services.Transactions.GetTransactionByTransactionId(env.Ctx, aitest.Uid, transactionId)
	assert.Equal(t, int64(101), transaction.CategoryId)

	result, err = callTool(t, env, "update_transaction", map[string]any{"id": id, "category_name": "Mercado", "tags": []string{"comida"}, "dry_run": false})
	assert.Nil(t, err)
	parsed = textOf(t, result)
	assert.Equal(t, false, parsed["dry_run"])
	assert.Equal(t, []any{"comida"}, parsed["after"].(map[string]any)["tags"])
	transaction, _ = services.Transactions.GetTransactionByTransactionId(env.Ctx, aitest.Uid, transactionId)
	assert.Equal(t, int64(102), transaction.CategoryId)

	_, err = callTool(t, env, "update_transaction", map[string]any{"id": id, "dry_run": true})
	assert.Equal(t, errs.ErrIncompleteOrIncorrectSubmission, err)

	_, err = callTool(t, env, "update_transaction", map[string]any{"id": id, "category_name": "Videojuegos", "dry_run": true})
	assert.Contains(t, err.Error(), "category \"Videojuegos\"")

	_, err = callTool(t, env, "update_transaction", map[string]any{"id": id, "category_name": "Mercado"})
	assert.Contains(t, err.Error(), "dry_run is required")

	_, err = callTool(t, env, "update_transaction", map[string]any{"id": "x", "category_name": "Mercado", "dry_run": true})
	assert.Equal(t, errs.ErrTransactionIdInvalid, err)
}

func TestGetSpendingSummary_Tool(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)

	rows := []any{
		expenseArgs("100.000", "Exito", "2026-10-05T12:00:00-05:00"),
		map[string]any{"type": "expense", "time": "2026-10-06T12:00:00-05:00", "category_name": "Taxi", "account_name": "Efectivo", "amount": "30.000", "comment": "Uber"},
		expenseArgs("200.000", "Exito", "2026-11-05T12:00:00-05:00"),
	}
	mustCall(t, env, "add_transactions_batch", batchArgs(rows, "seed", false))

	args := map[string]any{"start_time": "2026-10-01T00:00:00-05:00", "end_time": "2026-11-30T23:59:59-05:00", "type": "expense", "group_by": "category"}
	result, err := callTool(t, env, "get_spending_summary", args)
	assert.Nil(t, err)
	parsed := textOf(t, result)
	assert.Equal(t, "330000.00", parsed["grand_total"])
	assert.Equal(t, "COP", parsed["currency"])
	assert.Equal(t, map[string]any{"start": "2026-10-01T00:00:00-05:00", "end": "2026-11-30T23:59:59-05:00"}, parsed["period"])
	summaryRows := parsed["rows"].([]any)
	assert.Equal(t, map[string]any{"key": "Restaurantes", "total": "300000.00", "count": float64(2), "share": 0.9091}, summaryRows[0])
	assert.Equal(t, "Taxi", summaryRows[1].(map[string]any)["key"])
	assert.Equal(t, parsed["grand_total"], result.StructuredContent["grand_total"])

	args["group_by"] = "month"
	args["top"] = 1
	result, err = callTool(t, env, "get_spending_summary", args)
	assert.Nil(t, err)
	summaryRows = textOf(t, result)["rows"].([]any)
	assert.Equal(t, "2026-10", summaryRows[0].(map[string]any)["key"])
	assert.Equal(t, "2026-11", summaryRows[1].(map[string]any)["key"])

	for name, change := range map[string]map[string]any{
		"start":    {"start_time": "01/10/2026"},
		"end":      {"end_time": "hoy"},
		"type":     {"type": "transfer"},
		"group_by": {"group_by": "weekday"},
		"order":    {"start_time": "2026-12-01T00:00:00-05:00"},
	} {
		invalid := map[string]any{}

		for key, value := range args {
			invalid[key] = value
		}

		for key, value := range change {
			invalid[key] = value
		}

		_, err = callTool(t, env, "get_spending_summary", invalid)
		assert.NotNil(t, err, name)
	}
}

func TestQueryTransactionsReturnsTheTransactionId(t *testing.T) {
	initializeMCPContainerTest(t)
	env := aitest.NewEnv(t)

	imported := mustCall(t, env, "add_transactions_batch", batchArgs([]any{expenseArgs("45.900", "Éxito", "2026-10-15T12:00:00-05:00")}, "extracto", false))
	id := rowsOf(textOf(t, imported))[0].(map[string]any)["transaction_id"].(string)

	result, err := callTool(t, env, "query_transactions", map[string]any{"start_time": "2026-10-01T00:00:00-05:00", "end_time": "2026-10-31T23:59:59-05:00", "response_fields": "comment"})
	assert.Nil(t, err)
	transactions := textOf(t, result)["transactions"].([]any)
	assert.Equal(t, 1, len(transactions))
	assert.Equal(t, id, transactions[0].(map[string]any)["id"])
	assert.Equal(t, "Éxito [origen: extracto]", transactions[0].(map[string]any)["comment"])
}
