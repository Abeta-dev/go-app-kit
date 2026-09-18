package india

import (
	"fmt"
	"time"

	fintechin "github.com/umesh0492/go-fintech-india"
)

// FinancialQuarter represents Q1, Q2, Q3, or Q4 of the Indian Financial Year.
type FinancialQuarter string

const (
	// Q1 represents the first quarter (April - June).
	Q1 FinancialQuarter = "Q1"
	// Q2 represents the second quarter (July - September).
	Q2 FinancialQuarter = "Q2"
	// Q3 represents the third quarter (October - December).
	Q3 FinancialQuarter = "Q3"
	// Q4 represents the fourth quarter (January - March).
	Q4 FinancialQuarter = "Q4"
)

var istLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err == nil {
		return loc
	}
	return time.FixedZone("IST", 5*3600+30*60)
}()

// ISTLocation returns the *time.Location representing Asia/Kolkata (IST: UTC+5:30).
func ISTLocation() *time.Location {
	return istLocation
}

// FinancialYear contains Indian fiscal year details.
type FinancialYear struct {
	Label     string           // e.g., "FY 2026-27"
	ShortCode string           // e.g., "FY26-27"
	StartYear int              // e.g., 2026
	EndYear   int              // e.g., 2027
	StartDate time.Time        // April 1, 00:00:00 IST
	EndDate   time.Time        // March 31, 23:59:59 IST
	Quarter   FinancialQuarter // Q1, Q2, Q3, or Q4
}

// GetFinancialYear calculates the Indian Financial Year and Quarter for any given timestamp.
// Converts timestamps to Asia/Kolkata IST location before computing calendar year, month, or day boundaries
// to prevent 5.5h/day UTC shift errors.
// In India, the Financial Year begins on April 1st and ends on March 31st.
// Delegates statutory financial year calculation to Abeta go-fintech-india.
func GetFinancialYear(t time.Time) FinancialYear {
	tIST := t.In(istLocation)
	finFY := fintechin.FYFromDate(tIST)
	qNum := fintechin.Quarter(tIST)

	var q FinancialQuarter
	switch qNum {
	case 1:
		q = Q1
	case 2:
		q = Q2
	case 3:
		q = Q3
	case 4:
		q = Q4
	}

	startYear := finFY.Year
	endYear := startYear + 1
	shortStart := startYear % 100
	shortEnd := endYear % 100

	start := time.Date(startYear, time.April, 1, 0, 0, 0, 0, istLocation)
	end := time.Date(endYear, time.March, 31, 23, 59, 59, 999999999, istLocation)

	return FinancialYear{
		Label:     finFY.Label(),
		ShortCode: fmt.Sprintf("FY%02d-%02d", shortStart, shortEnd),
		StartYear: startYear,
		EndYear:   endYear,
		StartDate: start,
		EndDate:   end,
		Quarter:   q,
	}
}

// CurrentFinancialYear returns the Indian Financial Year for the current moment in time.
func CurrentFinancialYear() FinancialYear {
	return GetFinancialYear(time.Now())
}
