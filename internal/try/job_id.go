package try

import (
	"errors"
	"regexp"
	"strings"
)

var (
	uuidRegex = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	slugRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// IsJobID checks whether an ID is a valid UUID v4 format.
func IsJobID(id string) bool {
	return uuidRegex.MatchString(strings.TrimSpace(id))
}

// IsFeaturedSlug checks whether an identifier should be treated as a pre-curated review slug.
func IsFeaturedSlug(id string) bool {
	clean := strings.TrimSpace(id)
	return clean != "" && !IsJobID(clean)
}

// ValidateJobID ensures that the provided string conforms to a valid UUID format.
func ValidateJobID(id string) error {
	if !IsJobID(id) {
		return errors.New("invalid_job_id: malformed job UUID")
	}
	return nil
}

// ValidateSlug ensures that the provided slug conforms to standard URL-safe slug conventions.
func ValidateSlug(slug string) error {
	clean := strings.TrimSpace(slug)
	if clean == "" {
		return errors.New("invalid_slug: slug cannot be empty")
	}
	if len(clean) > 128 {
		return errors.New("invalid_slug: slug exceeds maximum length of 128 characters")
	}
	if !slugRegex.MatchString(clean) {
		return errors.New("invalid_slug: slug must consist of lowercase alphanumeric characters separated by hyphens")
	}
	return nil
}
