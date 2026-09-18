package india

import (
	"fmt"
	"strings"

	fintechin "github.com/umesh0492/go-fintech-india"
)

var (
	// ErrInvalidPANLength indicates the PAN string does not have length 10.
	ErrInvalidPANLength = fintechin.ErrInvalidPANLength
	// ErrInvalidPANFormat indicates the PAN format regex is invalid.
	ErrInvalidPANFormat = fintechin.ErrInvalidPANFormat
	// ErrUnknownEntityType indicates the 4th character of PAN is not a recognized entity type.
	ErrUnknownEntityType = fintechin.ErrInvalidPANEntityType
	// ErrInvalidPANEntityType is an alias for ErrUnknownEntityType.
	ErrInvalidPANEntityType = fintechin.ErrInvalidPANEntityType

	// Mapping of 4th character of PAN to legal entity type in India
	entityTypes = map[byte]string{
		'A': "Association of Persons (AOP)",
		'B': "Body of Individuals (BOI)",
		'C': "Company",
		'F': "Firm / Limited Liability Partnership (LLP)",
		'G': "Government Agency",
		'H': "Hindu Undivided Family (HUF)",
		'J': "Artificial Juridical Person",
		'L': "Local Authority",
		'P': "Individual (Person)",
		'T': "Trust",
	}
)

// PANDetails represents parsed information from a valid PAN.
type PANDetails struct {
	PAN            string
	EntityTypeCode string
	EntityTypeName string
	SurnameInitial string
}

// ValidatePAN verifies the structural validity of a 10-character Indian PAN card number.
// Delegates statutory validation to Abeta go-fintech-india.
func ValidatePAN(pan string) error {
	clean := strings.ToUpper(strings.TrimSpace(pan))
	if len(clean) != 10 {
		return ErrInvalidPANLength
	}

	if err := fintechin.ValidatePAN(clean); err != nil {
		return ErrInvalidPANFormat
	}

	fourthChar := clean[3]
	if _, err := fintechin.EntityType(clean); err != nil {
		return fmt.Errorf("%w: '%c' is not a valid PAN entity type", ErrUnknownEntityType, fourthChar)
	}

	return nil
}

// IsValidPAN returns true if the PAN passes format and entity checks.
func IsValidPAN(pan string) bool {
	return ValidatePAN(pan) == nil
}

// ParsePAN validates and parses the PAN card components.
func ParsePAN(pan string) (*PANDetails, error) {
	if err := ValidatePAN(pan); err != nil {
		return nil, err
	}
	clean := strings.ToUpper(strings.TrimSpace(pan))
	fourthChar := clean[3]

	return &PANDetails{
		PAN:            clean,
		EntityTypeCode: string(fourthChar),
		EntityTypeName: entityTypes[fourthChar],
		SurnameInitial: string(clean[4]),
	}, nil
}
