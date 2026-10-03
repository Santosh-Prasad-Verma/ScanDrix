package netguard_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/netguard"
)

func TestValidateRejectsMetadataAndInternalTargets(t *testing.T) {
	// These are the URLs that turn an outbound request into an SSRF primitive.
	// The cloud metadata address is the important one: it hands out instance
	// credentials to anything that asks, from inside the trust boundary.
	rejected := []string{
		"http://169.254.169.254/latest/meta-data/iam/security-credentials/",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://127.0.0.1:8080/admin",
		"http://localhost:6379/",
		"http://10.0.0.5/internal",
		"http://[::1]:9000/",
		"http://evil.test/x",
		"file:///etc/passwd",
		"gopher://127.0.0.1:11211/",
		"ftp://example.test/",
		"https://evil.test@trusted.test/x",
		"",
		"   ",
		"not-a-url",
		"/relative/path",
	}

	for _, raw := range rejected {
		t.Run(raw, func(t *testing.T) {
			if _, err := netguard.Validate(raw, netguard.Options{}); err == nil {
				t.Fatalf("expected %q to be rejected", raw)
			}
		})
	}
}

func TestValidateAcceptsHTTPS(t *testing.T) {
	for _, raw := range []string{
		"https://api.github.com/repos/a/b/releases",
		"https://discord.com/api/webhooks/1/abc",
		"https://sub.example.test:8443/x?y=1",
	} {
		t.Run(raw, func(t *testing.T) {
			u, err := netguard.Validate(raw, netguard.Options{})
			if err != nil {
				t.Fatalf("expected %q to be accepted, got %v", raw, err)
			}
			if u.String() != raw {
				t.Fatalf("round trip changed the url: %q -> %q", raw, u.String())
			}
		})
	}
}

// Loopback http is only permitted when the caller opts in, which is what tests
// and local tooling need.
func TestValidateLoopbackHTTPRequiresOptIn(t *testing.T) {
	opts := netguard.Options{AllowHTTPLoopback: true}

	for _, raw := range []string{"http://127.0.0.1:1234/x", "http://localhost:1234/x", "http://[::1]:1234/x"} {
		if _, err := netguard.Validate(raw, opts); err != nil {
			t.Errorf("expected %q to be accepted with the loopback opt-in, got %v", raw, err)
		}
	}

	// The opt-in must not extend to non-loopback over plaintext.
	if _, err := netguard.Validate("http://evil.test/x", opts); err == nil {
		t.Error("the loopback opt-in must not permit http to a non-loopback host")
	}
}

func TestValidateHostAllowlist(t *testing.T) {
	opts := netguard.Options{AllowedHosts: []string{"api.github.com"}}

	if _, err := netguard.Validate("https://api.github.com/x", opts); err != nil {
		t.Errorf("allowlisted host must be accepted: %v", err)
	}
	for _, raw := range []string{
		"https://evil.test/x",
		"https://api.github.com.evil.test/x", // suffix trickery
		"https://sub.api.github.com/x",       // not an exact match
	} {
		if _, err := netguard.Validate(raw, opts); err == nil {
			t.Errorf("expected %q to be rejected by the exact-match allowlist", raw)
		}
	}
}

func TestValidateHostSuffixAllowsSubdomains(t *testing.T) {
	opts := netguard.Options{AllowHostSuffix: "discord.com"}

	for _, raw := range []string{
		"https://discord.com/api/webhooks/1/x",
		"https://canary.discord.com/api/webhooks/1/x",
	} {
		if _, err := netguard.Validate(raw, opts); err != nil {
			t.Errorf("expected %q to be accepted: %v", raw, err)
		}
	}
	for _, raw := range []string{
		"https://notdiscord.com/x",
		"https://discord.com.evil.test/x",
	} {
		if _, err := netguard.Validate(raw, opts); err == nil {
			t.Errorf("expected %q to be rejected by the suffix allowlist", raw)
		}
	}
}

// The redirect case is the one a single validation misses: an allowed host can
// answer 302 with a metadata URL.
func TestRedirectPolicyRevalidatesEveryHop(t *testing.T) {
	metadata := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("INSTANCE-CREDENTIALS"))
	}))
	defer metadata.Close()

	opts := netguard.Options{AllowedHosts: []string{"updates.test"}}

	// An allowed host that redirects to the metadata endpoint.
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, metadata.URL, http.StatusFound)
	}))
	defer redirector.Close()

	// The redirector is on 127.0.0.1, so point the allowlist at it to model
	// "first hop permitted, second hop forbidden".
	opts.AllowedHosts = []string{strings.TrimPrefix(redirector.URL, "http://")}

	client := netguard.NewClient(5, opts)
	resp, err := client.Get(redirector.URL + "/latest")
	if err == nil {
		defer resp.Body.Close()
		body := make([]byte, 64)
		n, _ := resp.Body.Read(body)
		t.Fatalf("expected the redirect to a non-allowlisted host to be refused, got %q", string(body[:n]))
	}
	if !strings.Contains(err.Error(), "refusing redirect") {
		t.Fatalf("expected a redirect refusal, got %v", err)
	}
}

func TestRedirectPolicyStopsHopChains(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/again", http.StatusFound)
	}))
	defer server.Close()

	opts := netguard.Options{AllowHTTPLoopback: true}
	client := netguard.NewClient(5, opts)

	if _, err := client.Get(server.URL + "/start"); err == nil {
		t.Fatal("expected an endless redirect chain to be stopped")
	}
}
