package errs

import "net/http"

// NormalSubcategoryInboxItem is intentionally far from the upstream sub categories (0-21) to avoid code clashes when merging upstream
const NormalSubcategoryInboxItem = 50

// NewInboxToolError returns an error whose message explains to the AI client what to fix in the arguments of a tool
func NewInboxToolError(message string) *Error {
	return NewNormalError(NormalSubcategoryInboxItem, 99, http.StatusBadRequest, message)
}

// Error codes related to inbox items
var (
	ErrInboxItemIdInvalid          = NewNormalError(NormalSubcategoryInboxItem, 0, http.StatusBadRequest, "inbox item id is invalid")
	ErrInboxItemNotFound           = NewNormalError(NormalSubcategoryInboxItem, 1, http.StatusBadRequest, "inbox item not found")
	ErrInboxItemKindInvalid        = NewNormalError(NormalSubcategoryInboxItem, 2, http.StatusBadRequest, "inbox item kind is invalid")
	ErrInboxItemStatusInvalid      = NewNormalError(NormalSubcategoryInboxItem, 3, http.StatusBadRequest, "inbox item status is invalid")
	ErrNoInboxItemFile             = NewNormalError(NormalSubcategoryInboxItem, 4, http.StatusBadRequest, "no inbox item file")
	ErrInboxItemFileIsEmpty        = NewNormalError(NormalSubcategoryInboxItem, 5, http.StatusBadRequest, "inbox item file is empty")
	ErrExceedMaxInboxItemFileSize  = NewNormalError(NormalSubcategoryInboxItem, 6, http.StatusBadRequest, "exceed the maximum size of inbox item file")
	ErrInboxItemFileTypeNotAllowed = NewNormalError(NormalSubcategoryInboxItem, 7, http.StatusBadRequest, "inbox item file type is not supported")
	ErrInboxItemNotPending         = NewNormalError(NormalSubcategoryInboxItem, 8, http.StatusBadRequest, "inbox item is not pending")
	ErrInboxItemCannotBeDeleted    = NewNormalError(NormalSubcategoryInboxItem, 9, http.StatusBadRequest, "inbox item can only be deleted when it is pending or discarded")
	ErrInboxItemFileNotExists      = NewNormalError(NormalSubcategoryInboxItem, 10, http.StatusNotFound, "inbox item file not exists")
	ErrInboxItemFileIsNotPreviewed = NewNormalError(NormalSubcategoryInboxItem, 11, http.StatusBadRequest, "inbox item has no file that can be read")
	ErrInboxDownloadTokenInvalid   = NewNormalError(NormalSubcategoryInboxItem, 12, http.StatusForbidden, "inbox download token is invalid or expired")
	ErrInboxItemStorageDisabled    = NewNormalError(NormalSubcategoryInboxItem, 13, http.StatusBadRequest, "inbox requires transaction pictures to be enabled")
)
