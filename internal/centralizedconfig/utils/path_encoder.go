// Package utils provides path segment encoding and multi-directory group folder naming for centralized configuration.
package utils

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// GroupPathJoiner is the delimiter used between encoded directory paths in a multi-directory group.
const GroupPathJoiner = "&"

// Common path validation errors.
var (
	ErrEmptyPathGroup     = errors.New("group must have at least one path")
	ErrInvalidPathSegment = errors.New("path cannot be empty, whitespace, or repository root")
	ErrDuplicatePath      = errors.New("duplicate path in group")
)

func normalizePath(p string) string {
	clean := strings.Trim(strings.TrimSpace(p), "/")
	return clean
}

// EncodePathSegment encodes slashes and percent signs to create safe folder names.
func EncodePathSegment(decoded string) (string, error) {
	norm := normalizePath(decoded)
	if norm == "" {
		return "", ErrInvalidPathSegment
	}
	s := strings.ReplaceAll(norm, "%", "%25")
	s = strings.ReplaceAll(s, "/", "%2F")
	return s, nil
}

// DecodePathSegment decodes escaped slashes and percent signs back to relative paths.
func DecodePathSegment(encoded string) string {
	// Replace %2F with / and %25 with %
	s := strings.ReplaceAll(encoded, "%2F", "/")
	s = strings.ReplaceAll(s, "%2f", "/")
	s = strings.ReplaceAll(s, "%25", "%")
	return normalizePath(s)
}

// ValidateGroupPaths checks that the paths list is non-empty, contains no duplicates or root paths.
func ValidateGroupPaths(paths []string) error {
	if len(paths) == 0 {
		return ErrEmptyPathGroup
	}
	seen := make(map[string]struct{}, len(paths))
	for _, raw := range paths {
		norm := normalizePath(raw)
		if norm == "" {
			return ErrInvalidPathSegment
		}
		if _, exists := seen[norm]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicatePath, norm)
		}
		seen[norm] = struct{}{}
	}
	return nil
}

// BuildGroupFolderName sorts, validates, and encodes multiple directory paths into a deterministic folder name.
func BuildGroupFolderName(paths []string) (string, error) {
	if err := ValidateGroupPaths(paths); err != nil {
		return "", err
	}

	normalized := make([]string, len(paths))
	for i, p := range paths {
		normalized[i] = normalizePath(p)
	}
	sort.Strings(normalized)

	encodedSegments := make([]string, len(normalized))
	for i, p := range normalized {
		enc, err := EncodePathSegment(p)
		if err != nil {
			return "", err
		}
		encodedSegments[i] = enc
	}

	return strings.Join(encodedSegments, GroupPathJoiner), nil
}

// ParseGroupFolderName decodes and validates a group folder name back to the list of relative directory paths.
func ParseGroupFolderName(folderName string) ([]string, error) {
	clean := strings.TrimSpace(folderName)
	if clean == "" {
		return nil, ErrEmptyPathGroup
	}

	segments := strings.Split(clean, GroupPathJoiner)
	decoded := make([]string, len(segments))
	for i, seg := range segments {
		dec := DecodePathSegment(seg)
		if dec == "" {
			return nil, ErrInvalidPathSegment
		}
		decoded[i] = dec
	}

	if err := ValidateGroupPaths(decoded); err != nil {
		return nil, err
	}
	sort.Strings(decoded)
	return decoded, nil
}
