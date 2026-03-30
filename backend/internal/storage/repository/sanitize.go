package repository

import (
	"regexp"
	"strings"
)

var validColumnName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func sanitizeSortColumn(column, fallback string, allowed []string) string {
	column = strings.TrimSpace(column)
	if column == "" {
		return fallback
	}
	if !validColumnName.MatchString(column) {
		return fallback
	}
	for _, a := range allowed {
		if strings.EqualFold(column, a) {
			return a
		}
	}
	return fallback
}

func sanitizeSortOrder(order string) string {
	if strings.EqualFold(strings.TrimSpace(order), "ASC") {
		return "ASC"
	}
	return "DESC"
}
