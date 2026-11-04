package aiimport

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mayswind/ezbookkeeping/pkg/utils"
)

func TestParseLocalizedAmount_Valid(t *testing.T) {
	testCases := []struct {
		input    string
		expected string // normalized text of the amount
	}{
		{"1.234.567", "1234567.00"},
		{"1.234.567,89", "1234567.89"},
		{"45.900", "45900.00"},
		{"45,900.00", "45900.00"},
		{"12.34", "12.34"},
		{"12,34", "12.34"},
		{"1,234", "1234.00"},
		{"$ 45.900", "45900.00"},
		{"$45.900", "45900.00"},
		{"COP 1.234.567", "1234567.00"},
		{"cop 1.234.567,50", "1234567.50"},
		{"45.900 COP", "45900.00"},
		{"$ 1.234.567,89 COP", "1234567.89"},
		{"45900", "45900.00"},
		{"45900.5", "45900.50"},
		{"45900,5", "45900.50"},
		{"0", "0.00"},
		{"0,5", "0.50"},
		{"0.5", "0.50"},
		{".5", "0.50"},
		{"7", "7.00"},
		{"100.000", "100000.00"},
		{"100,000", "100000.00"},
		{"1,234,567", "1234567.00"},
		{"1,234,567.89", "1234567.89"},
		{"1.234,5", "1234.50"},
		{"1 234 567", "1234567.00"},
		{"  45.900  ", "45900.00"},
		{"+45.900", "45900.00"},
		{"-45.900", "-45900.00"},
		{"-$ 1.200,50", "-1200.50"},
		{"−45.900", "-45900.00"},
		{"999.999.999", "999999999.00"},
		{"1234.56", "1234.56"},
		{"1234,56", "1234.56"},
		{"12.000.000", "12000000.00"},
		{"2.500,00", "2500.00"},
		{"2,500.00", "2500.00"},
		{"10.50", "10.50"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.input, func(t *testing.T) {
			actual, err := ParseLocalizedAmount(testCase.input)
			assert.Nil(t, err)
			assert.Equal(t, testCase.expected, utils.FormatAmount(actual))
		})
	}
}

func TestParseLocalizedAmount_Ambiguous(t *testing.T) {
	testCases := []string{
		"",
		" ",
		"$",
		"-",
		"abc",
		"12a",
		"1.2.3",
		"1.23.456",
		"1,2,3",
		"1.234,567",
		"12.3456",
		"12,3456",
		"0.123",
		"0,123",
		"1234.567",
		"01.234",
		"12.",
		"12,",
		"1.234.56",
		"(45.900)",
		"45.900 pesos",
		"1.234.567.890.123.456.789",
		"1e5",
		"1,234.567,89",
		"99999999999999999",
	}

	for _, input := range testCases {
		t.Run(input, func(t *testing.T) {
			actual, err := ParseLocalizedAmount(input)
			assert.NotNil(t, err)
			assert.Equal(t, int64(0), actual)
			assert.Contains(t, err.Error(), "without thousands separators")
		})
	}
}
