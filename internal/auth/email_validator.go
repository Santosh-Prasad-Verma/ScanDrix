package auth

import (
	"errors"
	"net/mail"
	"strings"
)

var (
	// ErrInvalidEmailFormat indicates the email string does not conform to RFC 5322 syntax or exceeds RFC 5321 limits.
	ErrInvalidEmailFormat = errors.New("invalid email address format")
	// ErrDisposableEmailNotAllowed indicates the email domain is associated with a disposable/temporary inbox.
	ErrDisposableEmailNotAllowed = errors.New("disposable or temporary email addresses are not permitted")
)

// Known disposable email provider domains.
var defaultDisposableDomains = map[string]struct{}{
	"mailinator.com":         {},
	"tempmail.com":           {},
	"temp-mail.org":          {},
	"10minutemail.com":       {},
	"10minutemail.net":       {},
	"guerrillamail.com":      {},
	"guerrillamail.net":      {},
	"guerrillamail.org":      {},
	"guerrillamailblock.com": {},
	"sharklasers.com":        {},
	"throwawaymail.com":      {},
	"yopmail.com":            {},
	"yopmail.fr":             {},
	"yopmail.net":            {},
	"trashmail.com":          {},
	"trashmail.net":          {},
	"dispostable.com":        {},
	"getairmail.com":         {},
	"mohmal.com":             {},
	"fakeinbox.com":          {},
	"mytemp.email":           {},
	"crazymailing.com":       {},
	"nada.ltd":               {},
	"getnada.com":            {},
	"inboxkitten.com":        {},
	"burnermail.io":          {},
	"tempinbox.com":          {},
	"generator.email":        {},
	"emailondeck.com":        {},
	"tempail.com":            {},
}

// matchesDomain returns true if domain equals target or is a subdomain of target.
func matchesDomain(domain, target string) bool {
	cleaned := strings.ToLower(strings.TrimSpace(target))
	if cleaned == "" {
		return false
	}
	return domain == cleaned || strings.HasSuffix(domain, "."+cleaned)
}

// ValidateRegistrationEmail checks email syntax, strictly rejects display-name wrappers,
// enforces RFC 5321 length bounds, blocks disposable domains, and returns the canonicalized address.
func ValidateRegistrationEmail(email string, customBlockedDomains []string) (string, error) {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return "", ErrInvalidEmailFormat
	}

	// RFC 5321: Maximum path/email length is 254 octets
	if len(trimmed) > 254 {
		return "", ErrInvalidEmailFormat
	}

	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", ErrInvalidEmailFormat
	}

	// Strict display-name rejection: prevent email header smuggling (e.g. "Name <victim@gmail.com>" or "<victim@gmail.com>")
	if addr.Name != "" || strings.Contains(trimmed, "<") || strings.Contains(trimmed, ">") {
		return "", ErrInvalidEmailFormat
	}

	// Robust domain extraction handling quoted local parts with @ (RFC 5322)
	atIdx := strings.LastIndex(addr.Address, "@")
	if atIdx <= 0 || atIdx == len(addr.Address)-1 {
		return "", ErrInvalidEmailFormat
	}

	localPart := addr.Address[:atIdx]
	domainPart := strings.ToLower(strings.TrimSpace(addr.Address[atIdx+1:]))

	// RFC 5321 bounds: local-part <= 64 octets, domain <= 255 octets
	if len(localPart) > 64 || len(domainPart) > 255 {
		return "", ErrInvalidEmailFormat
	}

	// Domain must contain at least one dot separating labels (e.g. example.com)
	if !strings.Contains(domainPart, ".") {
		return "", ErrInvalidEmailFormat
	}

	// Check against built-in disposable list (matching root domain and subdomains)
	for disposable := range defaultDisposableDomains {
		if matchesDomain(domainPart, disposable) {
			return "", ErrDisposableEmailNotAllowed
		}
	}

	// Check against custom blocked domains
	for _, blocked := range customBlockedDomains {
		if matchesDomain(domainPart, blocked) {
			return "", ErrDisposableEmailNotAllowed
		}
	}

	// Return canonicalized email (normalized domain, lowercase)
	origAt := strings.LastIndex(trimmed, "@")
	origLocal := trimmed[:origAt]
	canonical := origLocal + "@" + domainPart
	return canonical, nil
}
