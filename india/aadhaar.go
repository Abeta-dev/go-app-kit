package india

import (
	"errors"
	"fmt"
	"strings"

	fintechin "github.com/umesh0492/go-fintech-india"
)

var (
	// ErrInvalidAadhaarLength indicates the Aadhaar number is not 12 digits.
	ErrInvalidAadhaarLength = fintechin.ErrInvalidAadhaarLength
	// ErrInvalidAadhaarFormat indicates the Aadhaar number contains invalid characters.
	ErrInvalidAadhaarFormat = fintechin.ErrInvalidAadhaarFormat
	// ErrInvalidAadhaarPrefix indicates the Aadhaar number starts with 0 or 1.
	ErrInvalidAadhaarPrefix = fintechin.ErrAadhaarStartsWithZeroOrOne
	// ErrAadhaarStartsWithZeroOrOne is an alias for ErrInvalidAadhaarPrefix.
	ErrAadhaarStartsWithZeroOrOne = fintechin.ErrAadhaarStartsWithZeroOrOne
	// ErrInvalidAadhaarChecksum indicates the Aadhaar number fails the Verhoeff checksum.
	ErrInvalidAadhaarChecksum = fintechin.ErrInvalidAadhaarChecksum
)

// ValidateAadhaar verifies that an Aadhaar number is 12 digits, does not start with 0 or 1,
// and satisfies the official UIDAI Verhoeff dihedral D5 checksum.
// Delegates statutory validation to github.com/umesh0492/go-fintech-india.
func ValidateAadhaar(aadhaar string) error {
	clean := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(aadhaar), "-", ""), " ", "")
	if len(clean) != 12 {
		return ErrInvalidAadhaarLength
	}

	if clean[0] == '0' || clean[0] == '1' {
		return ErrInvalidAadhaarPrefix
	}

	if err := fintechin.ValidateAadhaar(clean); err != nil {
		switch {
		case errors.Is(err, fintechin.ErrInvalidAadhaarLength):
			return ErrInvalidAadhaarLength
		case errors.Is(err, fintechin.ErrAadhaarStartsWithZeroOrOne):
			return ErrInvalidAadhaarPrefix
		case errors.Is(err, fintechin.ErrInvalidAadhaarChecksum):
			return ErrInvalidAadhaarChecksum
		default:
			return ErrInvalidAadhaarFormat
		}
	}

	return nil
}

// IsValidAadhaar returns true if the Aadhaar number passes all UIDAI validation rules.
func IsValidAadhaar(aadhaar string) bool {
	return ValidateAadhaar(aadhaar) == nil
}

// MaskAadhaar formats an Aadhaar number with privacy masking (e.g., "XXXX-XXXX-1234").
func MaskAadhaar(aadhaar string) string {
	clean := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(aadhaar), "-", ""), " ", "")
	if len(clean) != 12 {
		return aadhaar
	}
	return fmt.Sprintf("XXXX-XXXX-%s", clean[8:])
}

// FormatAadhaar formats an Aadhaar into standard 4-digit groups (e.g., "1234 5678 9012").
// Delegates formatting to github.com/umesh0492/go-fintech-india.
func FormatAadhaar(aadhaar string) string {
	clean := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(aadhaar), "-", ""), " ", "")
	if len(clean) != 12 {
		return aadhaar
	}
	return fintechin.FormatAadhaar(clean)
}
