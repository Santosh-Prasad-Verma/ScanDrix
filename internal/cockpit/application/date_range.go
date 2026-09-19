package application

import (
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/scandrix/backend/internal/cockpit/domain"
)

var dateRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// AssertIsoDate verifies that a string adheres to YYYY-MM-DD format.
func AssertIsoDate(value, label string) error {
	if !dateRegex.MatchString(value) {
		return fmt.Errorf("invalid %s. Expected YYYY-MM-DD, got %q", label, value)
	}
	return nil
}

// PreviousPeriod represents the computed prior comparison window.
type PreviousPeriod struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// ComputePreviousPeriod calculates an identically sized window ending the day before startDate.
func ComputePreviousPeriod(startDate, endDate string) (PreviousPeriod, error) {
	if err := AssertIsoDate(startDate, "startDate"); err != nil {
		return PreviousPeriod{}, err
	}
	if err := AssertIsoDate(endDate, "endDate"); err != nil {
		return PreviousPeriod{}, err
	}

	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return PreviousPeriod{}, err
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return PreviousPeriod{}, err
	}

	duration := end.Sub(start)
	prevEnd := start.AddDate(0, 0, -1)
	prevStart := prevEnd.Add(-duration)

	return PreviousPeriod{
		StartDate: prevStart.Format("2006-01-02"),
		EndDate:   prevEnd.Format("2006-01-02"),
	}, nil
}

// ComputeTrend calculates the percentage change and qualitative direction.
func ComputeTrend(current, previous float64, directionOfImprovement string) domain.ComparisonDetail {
	var percentageChange float64
	trend := string(domain.TrendUnchanged)

	if previous > 0 {
		raw := ((current - previous) / previous) * 100.0
		percentageChange = math.Round(raw*100.0) / 100.0

		if percentageChange == 0 {
			trend = string(domain.TrendUnchanged)
		} else if directionOfImprovement == "up" {
			if percentageChange > 0 {
				trend = string(domain.TrendImproved)
			} else {
				trend = string(domain.TrendWorsened)
			}
		} else { // "down"
			if percentageChange < 0 {
				trend = string(domain.TrendImproved)
			} else {
				trend = string(domain.TrendWorsened)
			}
		}
	} else if current > 0 {
		percentageChange = 100.0
		if directionOfImprovement == "up" {
			trend = string(domain.TrendImproved)
		} else {
			trend = string(domain.TrendWorsened)
		}
	}

	return domain.ComparisonDetail{
		PercentageChange: percentageChange,
		Trend:            trend,
	}
}

// LastNCompleteWeeks finds the n complete ISO weeks (Mon–Sun) ending on or before endDate.
func LastNCompleteWeeks(endDate string, weeks int) (string, string, error) {
	if err := AssertIsoDate(endDate, "endDate"); err != nil {
		return "", "", err
	}

	d, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return "", "", err
	}

	weekday := d.Weekday() // 0 = Sunday, 1 = Monday, ...
	var lastCompleteSunday time.Time
	if weekday == time.Sunday {
		lastCompleteSunday = d
	} else {
		// Days back to previous Sunday
		daysBack := int(weekday)
		lastCompleteSunday = d.AddDate(0, 0, -daysBack)
	}

	start := lastCompleteSunday.AddDate(0, 0, -(weeks*7 - 1))
	return start.Format("2006-01-02"), lastCompleteSunday.Format("2006-01-02"), nil
}

// MonthRange represents a calendar month's start, end, and presentation label.
type MonthRange struct {
	MonthStart string `json:"month_start"`
	MonthEnd   string `json:"month_end"`
	Label      string `json:"label"`
}

// LastNMonths returns the n calendar months ending with the month of endDate, oldest first.
func LastNMonths(endDate string, n int) ([]MonthRange, error) {
	if err := AssertIsoDate(endDate, "endDate"); err != nil {
		return nil, err
	}

	base, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, err
	}

	var results []MonthRange
	baseYear, baseMonth, _ := base.Date()

	for i := n - 1; i >= 0; i-- {
		// Target month
		mTime := time.Date(baseYear, baseMonth, 1, 0, 0, 0, 0, time.UTC).AddDate(0, -i, 0)
		year, month, _ := mTime.Date()

		monthStart := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		monthEnd := monthStart.AddDate(0, 1, -1)

		results = append(results, MonthRange{
			MonthStart: monthStart.Format("2006-01-02"),
			MonthEnd:   monthEnd.Format("2006-01-02"),
			Label:      monthStart.Format("Jan"),
		})
	}

	return results, nil
}
