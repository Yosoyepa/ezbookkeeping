package aiimport

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

// DownloadTokenLifetime is the max lifetime of the download url of an inbox file
const DownloadTokenLifetime = 15 * time.Minute

var usedDownloadTokens = struct {
	sync.Mutex
	items map[string]int64
}{items: make(map[string]int64)}

func signDownloadToken(secretKey string, payload string) string {
	mac := hmac.New(sha256.New, []byte("inbox-download:"+secretKey))
	mac.Write([]byte(payload))

	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// CreateDownloadToken creates a signed token which allows to download the file of an inbox item once, until it expires
func CreateDownloadToken(secretKey string, uid int64, itemId int64, expiresAt time.Time) (string, error) {
	nonce := make([]byte, 8)

	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	payload := fmt.Sprintf("%d:%d:%d:%s", uid, itemId, expiresAt.Unix(), hex.EncodeToString(nonce))

	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + signDownloadToken(secretKey, payload), nil
}

// ParseDownloadToken verifies the signature and the expiry of the token and returns the user id and the inbox item id.
// The token is only valid once: it is consumed when it is parsed successfully
func ParseDownloadToken(secretKey string, token string, now time.Time) (int64, int64, error) {
	parts := strings.Split(token, ".")

	if len(parts) != 2 {
		return 0, 0, errs.ErrInboxDownloadTokenInvalid
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])

	if err != nil {
		return 0, 0, errs.ErrInboxDownloadTokenInvalid
	}

	payload := string(payloadBytes)

	if !hmac.Equal([]byte(signDownloadToken(secretKey, payload)), []byte(parts[1])) {
		return 0, 0, errs.ErrInboxDownloadTokenInvalid
	}

	items := strings.Split(payload, ":")

	if len(items) != 4 {
		return 0, 0, errs.ErrInboxDownloadTokenInvalid
	}

	uid, err1 := utils.StringToInt64(items[0])
	itemId, err2 := utils.StringToInt64(items[1])
	expiresAt, err3 := utils.StringToInt64(items[2])

	if err1 != nil || err2 != nil || err3 != nil || uid <= 0 || itemId <= 0 {
		return 0, 0, errs.ErrInboxDownloadTokenInvalid
	}

	if now.Unix() > expiresAt {
		return 0, 0, errs.ErrInboxDownloadTokenInvalid
	}

	usedDownloadTokens.Lock()
	defer usedDownloadTokens.Unlock()

	for usedToken, usedExpiresAt := range usedDownloadTokens.items {
		if now.Unix() > usedExpiresAt {
			delete(usedDownloadTokens.items, usedToken)
		}
	}

	if _, used := usedDownloadTokens.items[parts[1]]; used {
		return 0, 0, errs.ErrInboxDownloadTokenInvalid
	}

	usedDownloadTokens.items[parts[1]] = expiresAt

	return uid, itemId, nil
}
