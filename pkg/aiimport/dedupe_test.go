package aiimport

import (
	"strings"
	"testing"

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
	assert.True(t, CommentsMatch("", "anything"))
}

func TestIsInDuplicateWindow(t *testing.T) {
	assert.True(t, IsInDuplicateWindow(1000, 1000))
	assert.True(t, IsInDuplicateWindow(1000, 1000+86400))
	assert.True(t, IsInDuplicateWindow(1000+86400, 1000))
	assert.False(t, IsInDuplicateWindow(1000, 1000+86401))
}
