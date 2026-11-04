package aiimport

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// duplicateWindowSeconds is the max distance between the time of a row and the time of an existing transaction to consider it a duplicate
const duplicateWindowSeconds = 24 * 60 * 60

// maxCommentLength is the max length of the comment of a transaction
const maxCommentLength = 255

var originSuffixRegexp = regexp.MustCompile(`\s*\[origen:[^\]]*\]\s*$`)

// StripOriginSuffix returns the comment without the " [origen: ...]" suffix added by the AI tools
func StripOriginSuffix(comment string) string {
	return strings.TrimSpace(originSuffixRegexp.ReplaceAllString(comment, ""))
}

// HasOriginSuffix returns whether the comment ends with the " [origen: ...]" suffix added by the AI tools
func HasOriginSuffix(comment string) bool {
	return originSuffixRegexp.MatchString(comment)
}

// BuildComment returns the comment of the transaction with the origin suffix, the original comment is shortened if the result would be too long
func BuildComment(comment string, source string) string {
	source = sanitizeSource(source)
	suffix := fmt.Sprintf("[origen: %s]", source)
	comment = StripOriginSuffix(comment)

	if comment == "" {
		return suffix
	}

	maxCommentRunes := maxCommentLength - len([]rune(suffix)) - 1
	runes := []rune(comment)

	if len(runes) > maxCommentRunes {
		runes = runes[:maxCommentRunes]
	}

	return strings.TrimSpace(string(runes)) + " " + suffix
}

// SourceOfComment returns the source of the origin suffix of the comment, or empty if the comment has no origin suffix
func SourceOfComment(comment string) string {
	suffix := originSuffixRegexp.FindString(comment)

	if suffix == "" {
		return ""
	}

	suffix = strings.TrimSpace(suffix)
	suffix = strings.TrimPrefix(suffix, "[origen:")
	suffix = strings.TrimSuffix(suffix, "]")

	return strings.TrimSpace(suffix)
}

func sanitizeSource(source string) string {
	source = strings.Map(func(r rune) rune {
		if r == ']' || r == '[' {
			return ')'
		}

		if unicode.IsControl(r) {
			return ' '
		}

		return r
	}, source)
	source = strings.TrimSpace(source)

	runes := []rune(source)

	if len(runes) > 60 {
		runes = runes[:60]
	}

	return string(runes)
}

// NormalizeComment returns the comment without the origin suffix, in lower case, without accents and with only the characters [a-z0-9]
func NormalizeComment(comment string) string {
	var builder strings.Builder

	for _, r := range norm.NFD.String(strings.ToLower(StripOriginSuffix(comment))) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

// NormalizeMerchant returns the comment without the origin suffix, in lower case, without accents and with the words separated by one space,
// truncated to maxLength characters, it is used to group the transactions by merchant
func NormalizeMerchant(comment string, maxLength int) string {
	var builder strings.Builder
	lastIsSpace := true

	for _, r := range norm.NFD.String(strings.ToLower(StripOriginSuffix(comment))) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastIsSpace = false
		} else if !lastIsSpace && !unicode.Is(unicode.Mn, r) {
			builder.WriteRune(' ')
			lastIsSpace = true
		}
	}

	merchant := strings.TrimSpace(builder.String())

	if len(merchant) > maxLength {
		merchant = strings.TrimSpace(merchant[:maxLength])
	}

	return merchant
}

// CommentsMatch returns whether the normalized comments are equal or one is contained in the other
func CommentsMatch(comment1 string, comment2 string) bool {
	normalized1 := NormalizeComment(comment1)
	normalized2 := NormalizeComment(comment2)

	return strings.Contains(normalized1, normalized2) || strings.Contains(normalized2, normalized1)
}

// IsInDuplicateWindow returns whether the two transaction times (unix time in seconds) are at most 1 day apart
func IsInDuplicateWindow(unixTime1 int64, unixTime2 int64) bool {
	difference := unixTime1 - unixTime2

	if difference < 0 {
		difference = -difference
	}

	return difference <= duplicateWindowSeconds
}
