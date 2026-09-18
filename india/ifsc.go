package india

import (
	"errors"
	"strings"

	fintechin "github.com/umesh0492/go-fintech-india"
)

var (
	// ErrInvalidIFSCLength indicates the IFSC string does not have length 11.
	ErrInvalidIFSCLength = fintechin.ErrInvalidIFSCLength
	// ErrInvalidIFSCFormat indicates the IFSC format is invalid (4 bank letters + 0 + 6 branch alphanumeric).
	ErrInvalidIFSCFormat = errors.New("ifsc format is invalid (4 bank letters + 0 + 6 branch alphanumeric)")
	// ErrInvalidIFSCBankCode indicates the first 4 characters are not alphabetic.
	ErrInvalidIFSCBankCode = fintechin.ErrInvalidIFSCBankCode
	// ErrInvalidIFSCFifthChar indicates the 5th character is not '0'.
	ErrInvalidIFSCFifthChar = fintechin.ErrInvalidIFSCFifthChar
	// ErrInvalidIFSCBranchCode indicates the last 6 characters are not alphanumeric.
	ErrInvalidIFSCBranchCode = fintechin.ErrInvalidIFSCBranchCode
)

// ValidateIFSC checks the structural validity of an Indian Financial System Code.
// Delegates statutory validation to Abeta go-fintech-india.
func ValidateIFSC(code string) error {
	clean := strings.ToUpper(strings.TrimSpace(code))
	if len(clean) != 11 {
		return ErrInvalidIFSCLength
	}

	if err := fintechin.ValidateIFSC(clean); err != nil {
		if errors.Is(err, fintechin.ErrInvalidIFSCLength) {
			return ErrInvalidIFSCLength
		}
		return ErrInvalidIFSCFormat
	}

	return nil
}

// IsValidIFSC returns true if the IFSC code matches RBI format.
func IsValidIFSC(code string) bool {
	return ValidateIFSC(code) == nil
}

// GetBankCode extracts the 4-letter bank identifier prefix from an IFSC code.
// Delegates extraction to Abeta go-fintech-india.
func GetBankCode(code string) (string, error) {
	if err := ValidateIFSC(code); err != nil {
		return "", err
	}
	clean := strings.ToUpper(strings.TrimSpace(code))
	return fintechin.BankCode(clean), nil
}

// GetBranchCode extracts the 6-character branch identifier from an IFSC code.
// Delegates extraction to Abeta go-fintech-india.
func GetBranchCode(code string) (string, error) {
	if err := ValidateIFSC(code); err != nil {
		return "", err
	}
	clean := strings.ToUpper(strings.TrimSpace(code))
	return fintechin.BranchCode(clean), nil
}
