package models

import (
	"strings"

	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// InboxItemKind represents the kind of an inbox item
type InboxItemKind byte

// Inbox item kinds
const (
	INBOX_ITEM_KIND_RECEIPT   InboxItemKind = 1
	INBOX_ITEM_KIND_STATEMENT InboxItemKind = 2
)

// InboxItemStatus represents the status of an inbox item
type InboxItemStatus byte

// Inbox item statuses
const (
	INBOX_ITEM_STATUS_PENDING   InboxItemStatus = 1
	INBOX_ITEM_STATUS_PROCESSED InboxItemStatus = 2
	INBOX_ITEM_STATUS_DISCARDED InboxItemStatus = 3
)

// Limits of the inbox
const (
	InboxItemStatementMaxFileSize = 20 * 1024 * 1024
)

// InboxStatementFileExtensions contains the file extensions (and content types) allowed for statements
var InboxStatementFileExtensions = map[string]string{
	"pdf":  "application/pdf",
	"csv":  "text/csv",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"xls":  "application/vnd.ms-excel",
	"txt":  "text/plain",
	"ofx":  "application/x-ofx",
	"qif":  "application/x-qif",
}

// InboxItem represents an uploaded receipt or statement waiting to be processed, stored in database
type InboxItem struct {
	Uid                  int64           `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created) NOT NULL"`
	Deleted              bool            `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created) NOT NULL"`
	Status               InboxItemStatus `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created) NOT NULL"`
	ItemId               int64           `xorm:"PK"`
	Kind                 InboxItemKind   `xorm:"NOT NULL"`
	PictureId            int64
	FileExtension        string `xorm:"VARCHAR(10) NOT NULL"`
	OriginalFileName     string `xorm:"VARCHAR(255)"`
	Note                 string `xorm:"VARCHAR(255)"`
	ResultTransactionIds string `xorm:"TEXT"`
	ProcessedNote        string `xorm:"VARCHAR(1024)"`
	CreatedIp            string `xorm:"VARCHAR(39)"`
	CreatedUnixTime      int64  `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created)"`
	UpdatedUnixTime      int64
	ProcessedUnixTime    int64
	DeletedUnixTime      int64
}

// InboxItemListRequest represents all parameters of inbox item listing request
type InboxItemListRequest struct {
	Status InboxItemStatus `form:"status" binding:"min=0,max=3"`
	Kind   InboxItemKind   `form:"kind" binding:"min=0,max=2"`
	Count  int32           `form:"count" binding:"required,min=1,max=50"`
	Page   int32           `form:"page" binding:"required,min=1"`
}

// InboxItemGetRequest represents all parameters of inbox item getting request
type InboxItemGetRequest struct {
	Id int64 `form:"id,string" binding:"required,min=1"`
}

// InboxItemDiscardRequest represents all parameters of inbox item discarding request
type InboxItemDiscardRequest struct {
	Id   int64  `json:"id,string" binding:"required,min=1"`
	Note string `json:"note" binding:"max=1024"`
}

// InboxItemReopenRequest represents all parameters of inbox item reopening request
type InboxItemReopenRequest struct {
	Id int64 `json:"id,string" binding:"required,min=1"`
}

// InboxItemDeleteRequest represents all parameters of inbox item deleting request
type InboxItemDeleteRequest struct {
	Id int64 `json:"id,string" binding:"required,min=1"`
}

// InboxItemInfoResponse represents a view-object of inbox item
type InboxItemInfoResponse struct {
	Id                   int64           `json:"id,string"`
	Kind                 InboxItemKind   `json:"kind"`
	Status               InboxItemStatus `json:"status"`
	PictureId            int64           `json:"pictureId,string,omitempty"`
	PictureUrl           string          `json:"pictureUrl,omitempty"`
	FileExtension        string          `json:"fileExtension"`
	OriginalFileName     string          `json:"originalFileName,omitempty"`
	Note                 string          `json:"note,omitempty"`
	ResultTransactionIds []string        `json:"resultTransactionIds"`
	ProcessedNote        string          `json:"processedNote,omitempty"`
	CreatedTime          int64           `json:"createdTime"`
	ProcessedTime        int64           `json:"processedTime,omitempty"`
}

// InboxItemListResponse represents a view-object of inbox item list
type InboxItemListResponse struct {
	Items      []*InboxItemInfoResponse `json:"items"`
	TotalCount int64                    `json:"totalCount"`
}

// InboxItemStatsResponse represents a view-object of the pending counts of the inbox
type InboxItemStatsResponse struct {
	PendingReceipts   int64 `json:"pendingReceipts"`
	PendingStatements int64 `json:"pendingStatements"`
}

// GetResultTransactionIds returns the ids of the transactions created from this item
func (i *InboxItem) GetResultTransactionIds() []int64 {
	ids := make([]int64, 0)

	if i.ResultTransactionIds == "" {
		return ids
	}

	for _, idStr := range strings.Split(i.ResultTransactionIds, ",") {
		id, err := utils.StringToInt64(strings.TrimSpace(idStr))

		if err == nil && id > 0 {
			ids = append(ids, id)
		}
	}

	return ids
}

// FormatInboxItemResultTransactionIds returns the textual representation of the transaction ids stored in an inbox item, all the ids are kept
func FormatInboxItemResultTransactionIds(transactionIds []int64) string {
	items := make([]string, 0, len(transactionIds))

	for _, id := range transactionIds {
		items = append(items, utils.Int64ToString(id))
	}

	return strings.Join(items, ",")
}

// MergeInboxItemResultTransactionIds returns the ids of the first list followed by the ids of the second list which are not in the first one
func MergeInboxItemResultTransactionIds(previousIds []int64, newIds []int64) []int64 {
	merged := make([]int64, 0, len(previousIds)+len(newIds))
	seen := make(map[int64]bool, len(previousIds)+len(newIds))

	for _, ids := range [][]int64{previousIds, newIds} {
		for _, id := range ids {
			if seen[id] {
				continue
			}

			seen[id] = true
			merged = append(merged, id)
		}
	}

	return merged
}

// ToInboxItemInfoResponse returns a view-object according to database model
func (i *InboxItem) ToInboxItemInfoResponse(pictureUrl string) *InboxItemInfoResponse {
	resultTransactionIds := make([]string, 0)

	for _, id := range i.GetResultTransactionIds() {
		resultTransactionIds = append(resultTransactionIds, utils.Int64ToString(id))
	}

	return &InboxItemInfoResponse{
		Id:                   i.ItemId,
		Kind:                 i.Kind,
		Status:               i.Status,
		PictureId:            i.PictureId,
		PictureUrl:           pictureUrl,
		FileExtension:        i.FileExtension,
		OriginalFileName:     i.OriginalFileName,
		Note:                 i.Note,
		ResultTransactionIds: resultTransactionIds,
		ProcessedNote:        i.ProcessedNote,
		CreatedTime:          i.CreatedUnixTime,
		ProcessedTime:        i.ProcessedUnixTime,
	}
}
