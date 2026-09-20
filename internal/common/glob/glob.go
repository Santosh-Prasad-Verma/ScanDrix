package glob

import (
	"regexp"
	"strings"
	"sync"
)

const maxCacheSize = 500

var (
	cacheMu             sync.RWMutex
	caseSensitiveCache   = make(map[string]*regexp.Regexp)
	caseInsensitiveCache = make(map[string]*regexp.Regexp)
)

func globToRegex(pattern string, caseInsensitive bool) (*regexp.Regexp, error) {
	var sb strings.Builder
	if caseInsensitive {
		sb.WriteString("(?i)")
	}
	sb.WriteString("^")

	inGroup := false
	i := 0
	n := len(pattern)

	for i < n {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < n && pattern[i+1] == '*' {
				// ** matches any characters including /
				if i+2 < n && pattern[i+2] == '/' {
					sb.WriteString("(?:.*/)?")
					i += 3
					continue
				} else {
					sb.WriteString(".*")
					i += 2
					continue
				}
			} else {
				// * matches any character except /
				sb.WriteString("[^/]*")
				i++
				continue
			}
		case '?':
			sb.WriteString("[^/]")
			i++
			continue
		case '.', '+', '(', ')', '^', '$', '[', ']', '|':
			sb.WriteString(`\`)
			sb.WriteByte(c)
			i++
			continue
		case '{':
			inGroup = true
			sb.WriteString("(?:")
			i++
			continue
		case '}':
			if inGroup {
				sb.WriteString(")")
				inGroup = false
			} else {
				sb.WriteString(`\}`)
			}
			i++
			continue
		case ',':
			if inGroup {
				sb.WriteString("|")
			} else {
				sb.WriteString(",")
			}
			i++
			continue
		default:
			sb.WriteByte(c)
			i++
		}
	}

	sb.WriteString("$")
	return regexp.Compile(sb.String())
}

func getCachedMatcher(pattern string, caseInsensitive bool) *regexp.Regexp {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	cache := caseSensitiveCache
	if caseInsensitive {
		cache = caseInsensitiveCache
	}

	if re, ok := cache[pattern]; ok {
		return re
	}

	if len(cache) >= maxCacheSize {
		// Evict cache if too large
		for k := range cache {
			delete(cache, k)
		}
	}

	re, err := globToRegex(pattern, caseInsensitive)
	if err != nil {
		re = regexp.MustCompile("^$")
	}
	cache[pattern] = re
	return re
}

func NormalizeFilename(filename string) string {
	res := strings.ReplaceAll(filename, `\`, "/")
	for strings.HasPrefix(res, "./") {
		res = strings.TrimPrefix(res, "./")
	}
	for strings.HasPrefix(res, "/") {
		res = strings.TrimPrefix(res, "/")
	}
	return res
}

func IsFileMatchingGlob(filename string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	norm := NormalizeFilename(filename)
	for _, p := range patterns {
		matcher := getCachedMatcher(p, false)
		if matcher.MatchString(norm) {
			return true
		}
	}
	return false
}

func IsFileMatchingGlobCaseInsensitive(filename string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	norm := NormalizeFilename(filename)
	for _, p := range patterns {
		matcher := getCachedMatcher(p, true)
		if matcher.MatchString(norm) {
			return true
		}
	}
	return false
}

func ClearMatcherCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	caseSensitiveCache = make(map[string]*regexp.Regexp)
	caseInsensitiveCache = make(map[string]*regexp.Regexp)
}

func GetMatcherCacheSize() (int, int) {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return len(caseSensitiveCache), len(caseInsensitiveCache)
}
