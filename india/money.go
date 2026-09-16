package india

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	fintechin "github.com/umesh0492/go-fintech-india"
)

var (
	// ErrInvalidMoneyFormat is returned when parsing an invalid monetary string.
	ErrInvalidMoneyFormat = errors.New("invalid money format")
	// ErrDivisionByZero is returned when splitting or dividing money by non-positive divisor.
	ErrDivisionByZero = errors.New("division by zero in money calculation")
)

// Money represents a monetary value in Indian Rupees stored as an exact integer count of paise (1 INR = 100 paise).
// This eliminates IEEE-754 floating-point inaccuracies in financial and accounting operations.
// Delegates underlying monetary arithmetic to github.com/umesh0492/go-fintech-india.
type Money struct {
	inner fintechin.Money
}

// NewMoney creates a Money instance from an exact integer count of paise.
func NewMoney(paise int64) Money {
	return Money{inner: fintechin.NewMoney(paise)}
}

// NewMoneyFromRupees creates a Money instance from whole rupees (e.g. 500 -> 50,000 paise).
func NewMoneyFromRupees(rupees int64) Money {
	return Money{inner: fintechin.NewMoneyFromRupees(rupees)}
}

// NewMoneyFromFloat creates a Money instance by rounding a float64 amount in rupees to the nearest paise.
// e.g. 15000.50 -> 1500050 paise, 1.995 -> 200 paise.
func NewMoneyFromFloat(amount float64) Money {
	return Money{inner: fintechin.NewMoneyFromFloat(amount)}
}

// Paise returns the underlying monetary value in paise (minor units).
func (m Money) Paise() int64 {
	return m.inner.Paise()
}

// Rupees returns the value as whole rupees (truncated towards zero).
func (m Money) Rupees() int64 {
	return m.inner.Rupees()
}

// Float64 converts the Money value to a float64 in rupees (for interop and display).
func (m Money) Float64() float64 {
	return m.inner.Float64()
}

// IsZero reports whether the money amount is exactly zero.
func (m Money) IsZero() bool {
	return m.inner.IsZero()
}

// IsPositive reports whether the money amount is strictly greater than zero.
func (m Money) IsPositive() bool {
	return m.inner.IsPositive()
}

// IsNegative reports whether the money amount is strictly less than zero.
func (m Money) IsNegative() bool {
	return m.inner.IsNegative()
}

// Abs returns the absolute value of the Money amount.
func (m Money) Abs() Money {
	return Money{inner: m.inner.Abs()}
}

// Negate returns the negated Money value.
func (m Money) Negate() Money {
	return Money{inner: m.inner.Negate()}
}

// Add returns the sum m + other.
func (m Money) Add(other Money) Money {
	return Money{inner: m.inner.Add(other.inner)}
}

// Sub returns the difference m - other.
func (m Money) Sub(other Money) Money {
	return Money{inner: m.inner.Sub(other.inner)}
}

// Mul multiplies m by an integer factor.
func (m Money) Mul(factor int64) Money {
	return Money{inner: m.inner.Mul(factor)}
}

// MulBasisPoints multiplies m by basis points (1 basis point = 0.01% = 0.0001, 100 bps = 1%).
// e.g. for 9% GST (900 bps), 1000 INR (100000 paise) * 900 / 10000 = 90 INR (9000 paise).
func (m Money) MulBasisPoints(bps int64) Money {
	return Money{inner: fintechin.NewMoney(int64(math.Round(float64(m.inner.Paise()*bps) / 10000.0)))}
}

// Percentage computes rate% of m (e.g. 9.0 for 9% GST) with standard financial rounding.
func (m Money) Percentage(rate float64) Money {
	return Money{inner: fintechin.NewMoney(int64(math.Round(float64(m.inner.Paise()) * (rate / 100.0))))}
}

// Split divides the monetary value into n parts without losing any paise due to integer truncation.
// The sum of the resulting slice is guaranteed to equal m.
func (m Money) Split(n int) ([]Money, error) {
	if n <= 0 {
		return nil, ErrDivisionByZero
	}
	parts, err := m.inner.Split(n)
	if err != nil {
		return nil, err
	}
	result := make([]Money, len(parts))
	for i, p := range parts {
		result[i] = Money{inner: p}
	}
	return result, nil
}

// Format returns the standard Indian currency string (e.g. "12,34,567.89").
func (m Money) Format() string {
	return FormatINRPaise(m.inner.Paise())
}

// String implements fmt.Stringer, returning the formatted INR currency string.
func (m Money) String() string {
	return m.Format()
}

// Words returns the amount in words following the Indian numbering system.
func (m Money) Words() string {
	return AmountToWordsINR(m.inner.Rupees())
}

// MarshalJSON serializes Money as a JSON object containing integer paise, formatted string, and currency.
// This ensures that no IEEE-754 floating-point numbers are emitted on the wire.
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		AmountPaise int64  `json:"amount_paise"`
		Formatted   string `json:"formatted"`
		Currency    string `json:"currency"`
	}{
		AmountPaise: m.inner.Paise(),
		Formatted:   m.Format(),
		Currency:    "INR",
	})
}

// UnmarshalJSON unmarshals Money from:
// 1. Structured JSON object: {"amount_paise": 12345, "formatted": "123.45", "currency": "INR"} or {"paise": 12345}
// 2. Integer number in paise: 12345
// 3. String formatted currency: "123.45" or "12,34,567.89"
// 4. Decimal float number: 1234.50
func (m *Money) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "null" || s == "" {
		m.inner = fintechin.NewMoney(0)
		return nil
	}

	// 1. Structured JSON object
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		var obj struct {
			AmountPaise *int64  `json:"amount_paise"`
			Paise       *int64  `json:"paise"`
			Formatted   *string `json:"formatted"`
		}
		if err := json.Unmarshal(data, &obj); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidMoneyFormat, err)
		}
		if obj.AmountPaise != nil {
			m.inner = fintechin.NewMoney(*obj.AmountPaise)
			return nil
		}
		if obj.Paise != nil {
			m.inner = fintechin.NewMoney(*obj.Paise)
			return nil
		}
		if obj.Formatted != nil {
			parsed, err := fintechin.ParseINR(*obj.Formatted)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrInvalidMoneyFormat, err)
			}
			m.inner = parsed
			return nil
		}
		return ErrInvalidMoneyFormat
	}

	// 2. Quoted string or numeric representations delegated to fintechin.Money
	var fm fintechin.Money
	if err := json.Unmarshal(data, &fm); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMoneyFormat, err)
	}
	m.inner = fm
	return nil
}

// Value implements driver.Valuer to persist paise as int64.
func (m Money) Value() (driver.Value, error) {
	return m.inner.Value()
}

// Scan implements sql.Scanner to read paise from database driver.
func (m *Money) Scan(src any) error {
	return m.inner.Scan(src)
}
