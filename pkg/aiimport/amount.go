// Package aiimport contains the logic shared by the MCP tools that create transactions from receipts and statements
package aiimport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var currencyWordRegexp = regexp.MustCompile(`(?i)cop`)

// maxAmountDigits is the max count of digits (integer and decimals) that fit the amount range of the transactions
const maxAmountDigits = 17

func newAmbiguousAmountError(original string) error {
	return fmt.Errorf("the amount \"%s\" is ambiguous or invalid, send it again without thousands separators (e.g. 45900 or 45900.50)", original)
}

// ParseLocalizedAmount parses an amount written in Colombian or US format and returns the value in hundredths (the unit used by the transactions)
//   - "1.234.567" -> 1234567, "1.234.567,89" -> 1234567.89, "45.900" -> 45900, "45,900.00" -> 45900
//   - "12.34" -> 12.34, "12,34" -> 12.34, "1,234" -> 1234 (a separator followed by exactly 3 digits is a thousands separator)
//
// The currency symbol "$", the word "COP", spaces and the sign are accepted. When the format is not clear it returns an error
func ParseLocalizedAmount(input string) (int64, error) {
	cleaned := currencyWordRegexp.ReplaceAllString(input, "")
	cleaned = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '$' {
			return -1
		}

		if r == '−' { // unicode minus sign
			return '-'
		}

		return r
	}, cleaned)

	negative := false

	if strings.HasPrefix(cleaned, "-") {
		negative = true
		cleaned = cleaned[1:]
	} else if strings.HasPrefix(cleaned, "+") {
		cleaned = cleaned[1:]
	}

	if cleaned == "" {
		return 0, newAmbiguousAmountError(input)
	}

	for _, r := range cleaned {
		if (r < '0' || r > '9') && r != '.' && r != ',' {
			return 0, newAmbiguousAmountError(input)
		}
	}

	integerPart, decimalPart, ok := splitAmount(cleaned)

	if !ok {
		return 0, newAmbiguousAmountError(input)
	}

	if len(decimalPart) > 2 || len(integerPart)+2 > maxAmountDigits {
		return 0, newAmbiguousAmountError(input)
	}

	for len(decimalPart) < 2 {
		decimalPart += "0"
	}

	if integerPart == "" {
		integerPart = "0"
	}

	value, err := strconv.ParseInt(integerPart+decimalPart, 10, 64)

	if err != nil {
		return 0, newAmbiguousAmountError(input)
	}

	if negative {
		value = -value
	}

	return value, nil
}

// splitAmount returns the digits of the integer part and the decimal part of an amount which only has digits, dots and commas
func splitAmount(s string) (string, string, bool) {
	dots := strings.Count(s, ".")
	commas := strings.Count(s, ",")

	if dots == 0 && commas == 0 {
		return s, "", true
	}

	if dots > 0 && commas > 0 {
		lastDot := strings.LastIndex(s, ".")
		lastComma := strings.LastIndex(s, ",")
		decimalSeparator, thousandsSeparator := ".", ","
		decimalIndex := lastDot

		if lastComma > lastDot {
			decimalSeparator, thousandsSeparator = ",", "."
			decimalIndex = lastComma
		}

		if strings.Count(s, decimalSeparator) != 1 {
			return "", "", false
		}

		integerPart, decimalPart := s[:decimalIndex], s[decimalIndex+1:]

		if decimalPart == "" || !isValidThousandsGrouping(integerPart, thousandsSeparator) {
			return "", "", false
		}

		return strings.ReplaceAll(integerPart, thousandsSeparator, ""), decimalPart, true
	}

	separator := "."

	if commas > 0 {
		separator = ","
	}

	if strings.Count(s, separator) > 1 {
		if !isValidThousandsGrouping(s, separator) {
			return "", "", false
		}

		return strings.ReplaceAll(s, separator, ""), "", true
	}

	index := strings.Index(s, separator)
	left, right := s[:index], s[index+1:]

	if len(right) == 3 {
		// a single separator followed by exactly 3 digits is a thousands separator, but only when the left part looks like a group
		if len(left) >= 1 && len(left) <= 3 && left[0] != '0' {
			return left + right, "", true
		}

		return "", "", false
	}

	if len(right) == 1 || len(right) == 2 {
		return left, right, true
	}

	return "", "", false
}

// isValidThousandsGrouping returns whether s is made of one group of 1-3 digits (not starting with 0) followed by groups of exactly 3 digits
func isValidThousandsGrouping(s string, separator string) bool {
	groups := strings.Split(s, separator)

	for i, group := range groups {
		if i == 0 {
			if len(group) < 1 || len(group) > 3 || group[0] == '0' {
				return false
			}
		} else if len(group) != 3 {
			return false
		}

		for _, r := range group {
			if r < '0' || r > '9' {
				return false
			}
		}
	}

	return true
}
