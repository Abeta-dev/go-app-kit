package india

import (
	"errors"
	"fmt"
	"strings"

	fintechin "github.com/umesh0492/go-fintech-india"
)

var (
	// ErrInvalidGSTINLength indicates the GSTIN does not have exactly 15 characters.
	ErrInvalidGSTINLength = fintechin.ErrInvalidGSTINLength
	// ErrInvalidGSTINFormat indicates the GSTIN contains invalid characters or regex.
	ErrInvalidGSTINFormat = fintechin.ErrInvalidGSTINFormat
	// ErrInvalidGSTIN is an alias for ErrInvalidGSTINFormat.
	ErrInvalidGSTIN = fintechin.ErrInvalidGSTINFormat
	// ErrInvalidGSTINChecksum indicates the 15th character checksum mismatch.
	ErrInvalidGSTINChecksum = fintechin.ErrInvalidGSTINChecksum
	// ErrInvalidStateCode indicates the 2-digit state code is invalid.
	ErrInvalidStateCode = fintechin.ErrInvalidStateCode
)

// GSTINDetails represents parsed information from a valid GSTIN.
type GSTINDetails struct {
	GSTIN      string
	StateCode  string
	StateName  string
	PAN        string
	EntityNum  string
	CheckDigit string
}

// ValidateGSTIN checks length, regex format, state code, and the official mod-36 checksum.
// Delegates statutory validation to Abeta go-fintech-india.
func ValidateGSTIN(gstin string) error {
	clean := strings.ToUpper(strings.TrimSpace(gstin))
	if len(clean) != 15 {
		return ErrInvalidGSTINLength
	}

	if err := fintechin.ValidateGSTIN(clean); err != nil {
		switch {
		case errors.Is(err, fintechin.ErrInvalidGSTINLength):
			return ErrInvalidGSTINLength
		case errors.Is(err, fintechin.ErrInvalidStateCode):
			return fmt.Errorf("%w: %s", ErrInvalidStateCode, clean[:2])
		case errors.Is(err, fintechin.ErrInvalidGSTINChecksum):
			expected := CalculateGSTINCheckDigit(clean[:14])
			return fmt.Errorf("%w: expected %c, got %c", ErrInvalidGSTINChecksum, expected, clean[14])
		default:
			return ErrInvalidGSTINFormat
		}
	}

	return nil
}

// IsValidGSTIN returns true if the GSTIN passes all validation rules.
func IsValidGSTIN(gstin string) bool {
	return ValidateGSTIN(gstin) == nil
}

// ParseGSTIN validates and decomposes a GSTIN into its constituent components.
func ParseGSTIN(gstin string) (*GSTINDetails, error) {
	if err := ValidateGSTIN(gstin); err != nil {
		return nil, err
	}
	clean := strings.ToUpper(strings.TrimSpace(gstin))
	stateCode := fintechin.StateCode(clean)
	stateName, _ := fintechin.StateName(stateCode)

	return &GSTINDetails{
		GSTIN:      clean,
		StateCode:  stateCode,
		StateName:  stateName,
		PAN:        clean[2:12],
		EntityNum:  string(clean[12]),
		CheckDigit: string(clean[14]),
	}, nil
}

// CalculateGSTINCheckDigit computes the official mod-36 checksum character for the first 14 chars.
// Delegates calculation to Abeta go-fintech-india.
func CalculateGSTINCheckDigit(input14 string) byte {
	clean := strings.ToUpper(strings.TrimSpace(input14))
	check, err := fintechin.CalculateGSTINChecksum(clean)
	if err != nil {
		return '0'
	}
	return check
}
