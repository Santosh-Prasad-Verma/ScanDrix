package utils

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ThreadIdentifiers is a map of key-value identifiers for deterministic thread generation.
type ThreadIdentifiers map[string]any

// GeneratedThread holds the deterministic ID and associated thread metadata.
type GeneratedThread struct {
	ID       string         `json:"id"`
	Metadata map[string]any `json:"metadata"`
}

// ThreadOptions provides optional prefix, description and type configuration.
type ThreadOptions struct {
	Prefix      string
	Description string
	Type        string
}

func simpleHash(s string) string {
	var hash int32
	for i := 0; i < len(s); i++ {
		c := int32(s[i])
		hash = (hash << 5) - hash + c
	}
	abs := int64(hash)
	if abs < 0 {
		abs = -abs
	}
	return strconv.FormatInt(abs, 36)
}

func sortIdentifiers(identifiers ThreadIdentifiers) string {
	keys := make([]string, 0, len(identifiers))
	for k := range identifiers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s-%v", k, identifiers[k])
	}
	return strings.Join(parts, "-")
}

func validateIdentifiers(identifiers ThreadIdentifiers) error {
	count := len(identifiers)
	if count == 0 {
		return errors.New("at least 1 identifier is required")
	}
	if count > 5 {
		return fmt.Errorf("maximum 5 identifiers allowed, provided: %d", count)
	}
	for k, v := range identifiers {
		if v == nil || fmt.Sprintf("%v", v) == "" {
			return fmt.Errorf("identifier '%s' cannot be empty or nil", k)
		}
	}
	return nil
}

func validatePrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", nil
	}
	if len(prefix) > 3 {
		return "", fmt.Errorf("prefix '%s' exceeds 3 characters limit", prefix)
	}
	return prefix, nil
}

// GenerateThreadID generates a stable TR-[prefix-]hash identifier from 1-5 identifiers.
func GenerateThreadID(identifiers ThreadIdentifiers, prefix string) (string, error) {
	if err := validateIdentifiers(identifiers); err != nil {
		return "", err
	}
	validPrefix, err := validatePrefix(prefix)
	if err != nil {
		return "", err
	}

	sortedStr := sortIdentifiers(identifiers)
	hash := simpleHash(sortedStr)

	baseThreadID := fmt.Sprintf("TR-%s", hash)
	if validPrefix != "" {
		baseThreadID = fmt.Sprintf("TR-%s-%s", validPrefix, hash)
	}

	if len(baseThreadID) <= 32 {
		return baseThreadID, nil
	}

	prefixPart := "TR-"
	if validPrefix != "" {
		prefixPart = fmt.Sprintf("TR-%s-", validPrefix)
	}

	available := 32 - len(prefixPart)
	if available <= 0 {
		if validPrefix != "" {
			return fmt.Sprintf("TR-%s", validPrefix), nil
		}
		return "TR", nil
	}

	if len(hash) > available {
		hash = hash[:available]
	}
	return fmt.Sprintf("%s%s", prefixPart, hash), nil
}

// CreateThreadID creates a deterministic GeneratedThread container.
func CreateThreadID(identifiers ThreadIdentifiers, opts ThreadOptions) (*GeneratedThread, error) {
	threadType := opts.Type
	if threadType == "" {
		threadType = "thread"
	}

	tid, err := GenerateThreadID(identifiers, opts.Prefix)
	if err != nil {
		return nil, err
	}

	desc := opts.Description
	if desc == "" {
		desc = fmt.Sprintf("Thread %s", tid)
	}

	metadata := make(map[string]any, len(identifiers)+2)
	metadata["description"] = desc
	metadata["type"] = threadType
	for k, v := range identifiers {
		metadata[k] = v
	}

	return &GeneratedThread{
		ID:       tid,
		Metadata: metadata,
	}, nil
}
