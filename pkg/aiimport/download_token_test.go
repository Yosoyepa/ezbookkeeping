package aiimport

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDownloadToken_ValidAndSingleUse(t *testing.T) {
	now := time.Now()
	token, err := CreateDownloadToken("secret", 1001, 555, now.Add(DownloadTokenLifetime))
	assert.Nil(t, err)

	uid, itemId, err := ParseDownloadToken("secret", token, now)
	assert.Nil(t, err)
	assert.Equal(t, int64(1001), uid)
	assert.Equal(t, int64(555), itemId)

	// the second use is rejected
	_, _, err = ParseDownloadToken("secret", token, now)
	assert.NotNil(t, err)
}

func TestDownloadToken_Expired(t *testing.T) {
	now := time.Now()
	token, err := CreateDownloadToken("secret", 1001, 555, now.Add(DownloadTokenLifetime))
	assert.Nil(t, err)

	_, _, err = ParseDownloadToken("secret", token, now.Add(DownloadTokenLifetime+time.Second))
	assert.NotNil(t, err)

	// the lifetime is at most 15 minutes
	assert.LessOrEqual(t, DownloadTokenLifetime, 15*time.Minute)
}

func TestDownloadToken_Invalid(t *testing.T) {
	now := time.Now()
	token, _ := CreateDownloadToken("secret", 1001, 555, now.Add(time.Minute))
	parts := strings.Split(token, ".")

	otherToken, _ := CreateDownloadToken("secret", 1002, 556, now.Add(time.Minute))
	otherParts := strings.Split(otherToken, ".")

	for _, invalid := range []string{
		"",
		"abc",
		"a.b.c",
		parts[0] + ".",
		"." + parts[1],
		parts[0] + ".AAAA",
		otherParts[0] + "." + parts[1], // the payload of another token with this signature
		token + "x",
	} {
		_, _, err := ParseDownloadToken("secret", invalid, now)
		assert.NotNil(t, err, invalid)
	}

	// signed with another key
	wrongKeyToken, _ := CreateDownloadToken("other-secret", 1001, 555, now.Add(time.Minute))
	_, _, err := ParseDownloadToken("secret", wrongKeyToken, now)
	assert.NotNil(t, err)
}
