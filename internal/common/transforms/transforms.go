// Package transforms provides common data manipulation, string formatting, array algorithms, and numeric operations.
package transforms

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	specialCharsRegex = regexp.MustCompile(`[^\w\s]|_`)
	capitalLetterRegex = regexp.MustCompile(`([A-Z]+)`)
	whitespaceRegex    = regexp.MustCompile(`\s+`)
	durationRegex      = regexp.MustCompile(`(?:(\d+)d )?(?:(\d+)h )?(?:(\d+)m )?(?:(\d+)s)?`)
)

var accentMap = map[rune]rune{
	'á': 'a', 'é': 'e', 'í': 'i', 'ó': 'o', 'ú': 'u',
	'â': 'a', 'ê': 'e', 'î': 'i', 'ô': 'o', 'û': 'u',
	'ã': 'a', 'õ': 'o', 'ç': 'c', 'à': 'a', 'ä': 'a',
	'è': 'e', 'ë': 'e', 'ì': 'i', 'ï': 'i', 'ò': 'o',
	'ö': 'o', 'ù': 'u', 'ü': 'u',
	'Á': 'A', 'É': 'E', 'Í': 'I', 'Ó': 'O', 'Ú': 'U',
	'Â': 'A', 'Ê': 'E', 'Î': 'I', 'Ô': 'O', 'Û': 'U',
	'Ã': 'A', 'Õ': 'O', 'Ç': 'C', 'À': 'A', 'Ä': 'A',
	'È': 'E', 'Ë': 'E', 'Ì': 'I', 'Ï': 'I', 'Ò': 'O',
	'Ö': 'O', 'Ù': 'U', 'Ü': 'U',
}

// RemoveAccents converts accented Latin characters to their plain ASCII equivalents.
func RemoveAccents(str string) string {
	var sb strings.Builder
	for _, r := range str {
		if replacement, ok := accentMap[r]; ok {
			sb.WriteRune(replacement)
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// RemoveSpecialChars strips special punctuation and symbols, retaining only alphanumeric and whitespace.
func RemoveSpecialChars(str string) string {
	return specialCharsRegex.ReplaceAllString(str, "")
}

// ToSnakeCase converts camelCase, PascalCase, or spaced strings into snake_case.
func ToSnakeCase(str string) string {
	if str == "" {
		return ""
	}
	// Insert underscore before capital letters
	s := capitalLetterRegex.ReplaceAllString(str, "_$1")
	// Replace spaces with underscores
	s = whitespaceRegex.ReplaceAllString(s, "_")
	s = strings.ToLower(s)
	// Strip leading/trailing underscores and duplicate underscores
	s = strings.Trim(s, "_")
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	return s
}

// ToCamelCase converts a string with spaces, underscores, or dashes to camelCase.
func ToCamelCase(str string) string {
	words := strings.FieldsFunc(str, func(r rune) bool {
		return r == '_' || r == '-' || unicode.IsSpace(r)
	})
	if len(words) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(strings.ToLower(words[0]))
	for _, word := range words[1:] {
		if len(word) > 0 {
			sb.WriteString(strings.ToUpper(string(word[0])))
			if len(word) > 1 {
				sb.WriteString(strings.ToLower(word[1:]))
			}
		}
	}
	return sb.String()
}

// ToKebabCase converts a string to kebab-case.
func ToKebabCase(str string) string {
	snake := ToSnakeCase(str)
	return strings.ReplaceAll(snake, "_", "-")
}

// TransformKey normalizes a key by removing accents, special characters, and converting to snake_case.
func TransformKey(key string) string {
	return ToSnakeCase(RemoveSpecialChars(RemoveAccents(key)))
}

// TransformValue strips accents and special characters from a string value.
func TransformValue(val string) string {
	return RemoveSpecialChars(RemoveAccents(val))
}

// FormatString capitalizes the first letter of a string.
func FormatString(str string) string {
	if str == "" {
		return ""
	}
	r := []rune(str)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// FormatStringWithXX replaces "XX" placeholder with formatted float value.
func FormatStringWithXX(desc string, value float64) string {
	formatted := strings.Replace(fmt.Sprintf("%.2f", value), ".", ",", 1)
	return strings.ReplaceAll(desc, "XX", formatted)
}

// StringToSeconds parses duration strings like "2d 4h 15m 30s" into total seconds.
func StringToSeconds(str string) int64 {
	matches := durationRegex.FindStringSubmatch(str)
	if len(matches) < 5 {
		return 0
	}
	days, _ := strconv.ParseInt(matches[1], 10, 64)
	hours, _ := strconv.ParseInt(matches[2], 10, 64)
	minutes, _ := strconv.ParseInt(matches[3], 10, 64)
	seconds, _ := strconv.ParseInt(matches[4], 10, 64)
	return days*86400 + hours*3600 + minutes*60 + seconds
}

// SecondsToFormattedString formats total seconds into human-readable duration "2d 4h 15m 30s".
func SecondsToFormattedString(totalSeconds int64) string {
	if totalSeconds <= 0 {
		return "0s"
	}
	days := totalSeconds / 86400
	rem := totalSeconds % 86400
	hours := rem / 3600
	rem %= 3600
	minutes := rem / 60
	seconds := rem % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, " ")
}

// Chunk splits a slice of items into chunks of specified size.
func Chunk[T any](items []T, chunkSize int) [][]T {
	if chunkSize <= 0 || len(items) == 0 {
		return nil
	}
	var chunks [][]T
	for i := 0; i < len(items); i += chunkSize {
		end := i + chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[i:end])
	}
	return chunks
}

// Unique returns a new slice containing only distinct elements preserving order.
func Unique[T comparable](items []T) []T {
	seen := make(map[T]struct{}, len(items))
	result := make([]T, 0, len(items))
	for _, item := range items {
		if _, exists := seen[item]; !exists {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

// Intersection returns elements present in both slice A and slice B.
func Intersection[T comparable](a, b []T) []T {
	bMap := make(map[T]struct{}, len(b))
	for _, item := range b {
		bMap[item] = struct{}{}
	}
	var result []T
	for _, item := range a {
		if _, exists := bMap[item]; exists {
			result = append(result, item)
		}
	}
	return Unique(result)
}

// Difference returns elements present in slice A that are not in slice B.
func Difference[T comparable](a, b []T) []T {
	bMap := make(map[T]struct{}, len(b))
	for _, item := range b {
		bMap[item] = struct{}{}
	}
	var result []T
	for _, item := range a {
		if _, exists := bMap[item]; !exists {
			result = append(result, item)
		}
	}
	return Unique(result)
}

// Compact removes zero-value elements from slice.
func Compact[T comparable](items []T) []T {
	var zero T
	var result []T
	for _, item := range items {
		if item != zero {
			result = append(result, item)
		}
	}
	return result
}

// Flatten flattens a slice of slices into a single slice.
func Flatten[T any](slices [][]T) []T {
	var total int
	for _, s := range slices {
		total += len(s)
	}
	result := make([]T, 0, total)
	for _, s := range slices {
		result = append(result, s...)
	}
	return result
}

// Clamp restricts a value between min and max.
func Clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

// Round rounds a float to specified decimal places.
func Round(val float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Round(val*factor) / factor
}

// FormatBytes formats byte counts into human-readable B, KB, MB, GB strings.
func FormatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	val := float64(bytes)
	for _, unit := range units {
		val /= 1024
		if val < 1024 {
			return fmt.Sprintf("%.2f %s", val, unit)
		}
	}
	return fmt.Sprintf("%.2f PB", val)
}

// DeepClone clones an arbitrary struct or map via JSON round-trip.
func DeepClone[T any](src T) (T, error) {
	var dst T
	bytes, err := json.Marshal(src)
	if err != nil {
		return dst, err
	}
	err = json.Unmarshal(bytes, &dst)
	return dst, err
}

// DeepMerge merges src map recursively into dst map.
func DeepMerge(dst, src map[string]any) map[string]any {
	out := make(map[string]any, len(dst))
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		if srcMap, ok := v.(map[string]any); ok {
			if dstMap, ok := out[k].(map[string]any); ok {
				out[k] = DeepMerge(dstMap, srcMap)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// FormatISO8601 returns standard ISO 8601 / RFC 3339 formatted timestamp.
func FormatISO8601(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// ParseISO8601 parses standard ISO 8601 or RFC 3339 timestamp.
func ParseISO8601(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}
