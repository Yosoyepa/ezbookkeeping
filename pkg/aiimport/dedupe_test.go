package aiimport

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeComment(t *testing.T) {
	assert.Equal(t, "compraexitosuba", NormalizeComment("COMPRA Éxito - Suba"))
	assert.Equal(t, "exito", NormalizeComment("  éxito  [origen: bancolombia-2026-10]"))
	assert.Equal(t, "cafe123", NormalizeComment("Café #123"))
	assert.Equal(t, "nino", NormalizeComment("Niño")) // keeps the base letters of accented characters
	assert.Equal(t, "", NormalizeComment("[origen: inbox:123]"))
	assert.Equal(t, "", NormalizeComment("   "))
}

func TestStripOriginSuffix(t *testing.T) {
	assert.Equal(t, "Éxito Suba", StripOriginSuffix("Éxito Suba [origen: inbox:123]"))
	assert.Equal(t, "Éxito Suba", StripOriginSuffix("Éxito Suba"))
	assert.Equal(t, "", StripOriginSuffix("[origen: bancolombia-2026-10]"))
	assert.Equal(t, "a [b] c", StripOriginSuffix("a [b] c [origen: x]"))
}

func TestBuildComment(t *testing.T) {
	assert.Equal(t, "Éxito Suba [origen: inbox:123]", BuildComment("Éxito Suba", "inbox:123"))
	assert.Equal(t, "[origen: bancolombia-2026-10]", BuildComment("", "bancolombia-2026-10"))
	assert.Equal(t, "Éxito [origen: x]", BuildComment("Éxito [origen: old]", "x"))
	assert.Equal(t, "a [origen: x)y)]", BuildComment("a", "x]y["))

	long := BuildComment(strings.Repeat("ñ", 400), "inbox:123")
	assert.Equal(t, 255, len([]rune(long)))
	assert.True(t, strings.HasSuffix(long, " [origen: inbox:123]"))
}

func TestSourceOfComment(t *testing.T) {
	assert.Equal(t, "inbox:123", SourceOfComment("Éxito [origen: inbox:123]"))
	assert.Equal(t, "", SourceOfComment("Éxito"))
	assert.True(t, HasOriginSuffix("x [origen: y]"))
	assert.False(t, HasOriginSuffix("x"))
}

func TestNormalizeMerchant(t *testing.T) {
	assert.Equal(t, "compra exito suba", NormalizeMerchant("COMPRA Éxito - Suba [origen: inbox:1]", 40))
	assert.Equal(t, "compra exito suba", NormalizeMerchant("  compra   exito___suba ", 40))
	assert.Equal(t, "cafe", NormalizeMerchant("Café", 40))
	assert.Equal(t, "", NormalizeMerchant("[origen: x]", 40))
	assert.Equal(t, "compra exito", NormalizeMerchant("COMPRA EXITO SUBA CALLE 145", 12))
	assert.Equal(t, 39, len(NormalizeMerchant(strings.Repeat("abcd ", 20), 40))) // cut at 40 characters, the trailing space is trimmed
}

func TestCommentsMatch(t *testing.T) {
	assert.True(t, CommentsMatch("Éxito Suba", "COMPRA EXITO SUBA [origen: x]"))
	assert.True(t, CommentsMatch("exito", "Compra Éxito"))
	assert.True(t, CommentsMatch("Éxito", "éxito"))
	assert.False(t, CommentsMatch("Éxito", "Carulla"))

	// a comment without characters to compare says nothing about the merchant
	assert.False(t, CommentsMatch("", "anything"))
	assert.False(t, CommentsMatch("anything", "[origen: x]"))
	assert.False(t, CommentsMatch("", ""))
	assert.False(t, CommentsMatch("---", "Carulla"))
}

func TestIsDuplicateByComment(t *testing.T) {
	bogota := time.FixedZone("COT", -5*3600)
	at := func(day int, hour int) int64 {
		return time.Date(2026, 10, day, hour, 0, 0, 0, bogota).Unix()
	}

	// with comments on both sides, the window and the comments decide
	assert.True(t, IsDuplicateByComment("Éxito Suba", at(15, 12), "COMPRA EXITO SUBA [origen: x]", at(16, 8), bogota))
	assert.False(t, IsDuplicateByComment("Éxito", at(15, 12), "Carulla", at(15, 12), bogota))
	assert.False(t, IsDuplicateByComment("Éxito", at(15, 12), "Éxito", at(17, 12), bogota))

	// one comment empty: not a duplicate, whatever the other one says
	assert.False(t, IsDuplicateByComment("", at(15, 12), "Carulla", at(15, 12), bogota))
	assert.False(t, IsDuplicateByComment("Carulla", at(15, 12), "[origen: x]", at(15, 12), bogota))
	assert.False(t, IsDuplicateByComment("---", at(15, 12), "Carulla", at(15, 13), bogota))

	// both empty: only the same calendar day (in the location) is a duplicate
	assert.True(t, IsDuplicateByComment("", at(15, 8), "[origen: x]", at(15, 20), bogota))
	assert.False(t, IsDuplicateByComment("", at(15, 23), "", at(16, 6), bogota)) // 7 hours apart, but two days
	assert.False(t, IsDuplicateByComment("", at(15, 8), "", at(17, 8), bogota))  // out of the window
	assert.True(t, IsDuplicateByComment("", at(15, 23), "", at(16, 6), time.FixedZone("UTC+8", 8*3600)))
}

func TestIsSameCalendarDay(t *testing.T) {
	bogota := time.FixedZone("COT", -5*3600)
	unix := func(day int, hour int, location *time.Location) int64 {
		return time.Date(2026, 10, day, hour, 0, 0, 0, location).Unix()
	}

	assert.True(t, IsSameCalendarDay(unix(15, 0, bogota), unix(15, 23, bogota), bogota))
	assert.False(t, IsSameCalendarDay(unix(15, 23, bogota), unix(16, 0, bogota), bogota))
	assert.True(t, IsSameCalendarDay(unix(15, 23, bogota), unix(16, 0, bogota), nil)) // UTC: 04:00 and 05:00 of the 16th
}

func TestIsInDuplicateWindow(t *testing.T) {
	assert.True(t, IsInDuplicateWindow(1000, 1000))
	assert.True(t, IsInDuplicateWindow(1000, 1000+86400))
	assert.True(t, IsInDuplicateWindow(1000+86400, 1000))
	assert.False(t, IsInDuplicateWindow(1000, 1000+86401))
}
