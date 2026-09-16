package india

import (
	"strings"

	fintechin "github.com/umesh0492/go-fintech-india"
)

// FormatINRPaise formats an exact integer amount in paise (1 INR = 100 paise)
// into Indian currency format with comma placements and two decimal digits.
// e.g. 123456789 -> "12,34,567.89", -5000 -> "-50.00"
// Delegates formatting to github.com/umesh0492/go-fintech-india.
func FormatINRPaise(paise int64) string {
	return fintechin.FormatINR(paise)
}

// AmountToWordsINR converts an integer rupee amount into words following the Indian numbering system.
// Supports amounts up to arbitrary Crores (Crore, Lakh, Thousand, Hundred).
// Delegates word conversion to github.com/umesh0492/go-fintech-india.
func AmountToWordsINR(amount int64) string {
	if amount == 0 {
		return "Zero Rupees Only"
	}

	isNegative := amount < 0
	absAmount := amount
	if isNegative {
		absAmount = -absAmount
	}

	words := fintechin.NumberToIndianWords(absAmount)
	// Replace hyphens ("Twenty-Three" -> "Twenty Three") for standard Indian banking prose format
	words = strings.ReplaceAll(words, "-", " ")

	resStr := words + " Rupees Only"
	if isNegative {
		return "Minus " + resStr
	}
	return resStr
}
