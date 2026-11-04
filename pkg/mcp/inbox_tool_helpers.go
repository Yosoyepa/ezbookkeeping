package mcp

import (
	"encoding/json/v2"
	"strings"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

const dryRunDescription = "Call first with dry_run=true, show the result to the user and only call again with dry_run=false after explicit confirmation."

const (
	inboxKindReceiptName   = "receipt"
	inboxKindStatementName = "statement"

	inboxStatusPendingName   = "pending"
	inboxStatusProcessedName = "processed"
	inboxStatusDiscardedName = "discarded"
)

// inboxKindArgument is the kind of an inbox item as a model writes it: the name (receipt, statement), or the number of the web api (1, 2) as a string or a number
type inboxKindArgument string

// UnmarshalJSON accepts a json string or a json number
func (k *inboxKindArgument) UnmarshalJSON(data []byte) error {
	value := strings.TrimSpace(string(data))

	if value == "null" {
		return nil
	}

	if strings.HasPrefix(value, "\"") {
		var text string

		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}

		value = strings.TrimSpace(text)
	}

	*k = inboxKindArgument(value)

	return nil
}

func unmarshalToolArguments(callToolReq *MCPCallToolRequest, target any) error {
	if callToolReq.Arguments == nil {
		return errs.ErrIncompleteOrIncorrectSubmission
	}

	if err := json.Unmarshal(callToolReq.Arguments, target); err != nil {
		return errs.NewIncompleteOrIncorrectSubmissionError(err)
	}

	return nil
}

func newJSONTextResponse(response any) (any, []*MCPTextContent, error) {
	content, err := json.Marshal(response)

	if err != nil {
		return nil, nil, err
	}

	return response, []*MCPTextContent{
		NewMCPTextContent(string(content)),
	}, nil
}

func parseInboxItemIdArgument(id string) (int64, error) {
	itemId, err := utils.StringToInt64(id)

	if err != nil || itemId <= 0 {
		return 0, errs.ErrInboxItemIdInvalid
	}

	return itemId, nil
}

func requireDryRunArgument(dryRun *bool) (bool, error) {
	if dryRun == nil {
		return false, errs.NewInboxToolError("dry_run is required. " + dryRunDescription)
	}

	return *dryRun, nil
}

func getInboxKindName(kind models.InboxItemKind) string {
	if kind == models.INBOX_ITEM_KIND_RECEIPT {
		return inboxKindReceiptName
	}

	return inboxKindStatementName
}

func getInboxStatusName(status models.InboxItemStatus) string {
	switch status {
	case models.INBOX_ITEM_STATUS_PROCESSED:
		return inboxStatusProcessedName
	case models.INBOX_ITEM_STATUS_DISCARDED:
		return inboxStatusDiscardedName
	}

	return inboxStatusPendingName
}

func formatUnixTimeUTC(unixTime int64) string {
	if unixTime <= 0 {
		return ""
	}

	return time.Unix(unixTime, 0).UTC().Format(time.RFC3339)
}
