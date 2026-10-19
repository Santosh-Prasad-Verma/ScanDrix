package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderSlackSignature = "X-Slack-Signature"
	HeaderSlackTimestamp = "X-Slack-Request-Timestamp"
)

// VerifySlackSignature validates the incoming webhook request using Slack's HMAC-SHA256 protocol.
func VerifySlackSignature(signingSecret string, headers http.Header, body []byte) error {
	if signingSecret == "" {
		return nil // Permissive when signing secret is not configured in development
	}

	sig := headers.Get(HeaderSlackSignature)
	if sig == "" {
		sig = headers.Get(strings.ToLower(HeaderSlackSignature))
	}
	if sig == "" {
		return fmt.Errorf("missing %s header", HeaderSlackSignature)
	}

	tsStr := headers.Get(HeaderSlackTimestamp)
	if tsStr == "" {
		tsStr = headers.Get(strings.ToLower(HeaderSlackTimestamp))
	}
	if tsStr == "" {
		return fmt.Errorf("missing %s header", HeaderSlackTimestamp)
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid %s format: %w", HeaderSlackTimestamp, err)
	}

	// Guard against replay attacks (reject timestamps older than 5 minutes)
	diff := time.Now().UTC().Unix() - ts
	if diff < -300 || diff > 300 {
		return fmt.Errorf("request timestamp expired (%d seconds skew)", diff)
	}

	// Base string format: v0:<timestamp>:<body>
	baseStr := fmt.Sprintf("v0:%d:%s", ts, string(body))

	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(baseStr))
	expectedSignature := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expectedSignature)) {
		return fmt.Errorf("slack signature mismatch")
	}

	return nil
}
