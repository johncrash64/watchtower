package util

import (
	"fmt"
	"regexp"
	"strings"
)

var weekPattern = regexp.MustCompile(`^(\d{4})-W(\d{2})$`)

func ValidateWeekID(weekID string) error {
	if !weekPattern.MatchString(weekID) {
		return fmt.Errorf("invalid week format %q, expected YYYY-WNN", weekID)
	}
	return nil
}

func NormalizeWeekID(weekID string) (string, error) {
	weekID = strings.TrimSpace(strings.ToUpper(weekID))
	if err := ValidateWeekID(weekID); err != nil {
		return "", err
	}
	return weekID, nil
}
