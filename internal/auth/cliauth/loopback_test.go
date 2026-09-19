package cliauth_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/pkg/models"
)

func TestCLILoopbackFlow(t *testing.T) {
	ctx := context.Background()
	mgr := cliauth.NewLoopbackManager("https://app.scandrix.io")

	// 1. Launch local loopback HTTP server (CLI side)
	port, callbackChan, closeFn, err := cliauth.StartLocalLoopbackServer()
	if err != nil {
		t.Fatalf("failed starting local loopback server: %v", err)
	}
	defer closeFn()

	if port <= 0 {
		t.Fatalf("expected valid ephemeral port, got: %d", port)
	}

	// 2. Initiate loopback session on backend
	initRes, err := mgr.InitLoopback(ctx, port, "ScanDrix-CLI/v1.0")
	if err != nil {
		t.Fatalf("failed initiating loopback: %v", err)
	}
	u, err := url.Parse(initRes.VerificationURI)
	if err != nil {
		t.Fatalf("failed parsing verification URI: %v", err)
	}
	expectedRedirect := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	if initRes.State == "" || u.Query().Get("redirect_uri") != expectedRedirect {
		t.Fatalf("unexpected verification URI: %+v (query redirect_uri=%q)", initRes, u.Query().Get("redirect_uri"))
	}

	// 3. Poll pending state
	pollPending, err := mgr.PollLoopback(ctx, initRes.State)
	if err != nil || pollPending.Status != cliauth.StatusPending {
		t.Fatalf("expected pending status, got: %+v, err: %v", pollPending, err)
	}

	// 4. Simulate browser hitting local callback server: GET http://127.0.0.1:<port>/callback?state=<state>
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback?state=%s", port, initRes.State)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from callback server, got: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ScanDrix CLI Authorized") {
		t.Fatalf("expected callback HTML page, got: %s", string(body))
	}

	select {
	case receivedState := <-callbackChan:
		if receivedState != initRes.State {
			t.Fatalf("mismatched state received: got %s, want %s", receivedState, initRes.State)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback state")
	}

	// 5. Backend receives approval from web dashboard
	user := &models.AccountProfile{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
		Email:       "developer@acme.com",
	}
	err = mgr.CompleteLoopback(ctx, initRes.State, "access-token-jwt", "refresh-token-jwt", user)
	if err != nil {
		t.Fatalf("failed completing loopback: %v", err)
	}

	// 6. CLI polls backend to fetch tokens
	pollComplete, err := mgr.PollLoopback(ctx, initRes.State)
	if err != nil || pollComplete.Status != cliauth.StatusCompleted {
		t.Fatalf("expected completed status, got: %+v, err: %v", pollComplete, err)
	}
	if pollComplete.AccessToken != "access-token-jwt" || pollComplete.RefreshToken != "refresh-token-jwt" {
		t.Fatalf("tokens mismatch: %+v", pollComplete)
	}
	if pollComplete.UserEmail != "developer@acme.com" {
		t.Fatalf("email mismatch: %s", pollComplete.UserEmail)
	}

	// 7. Subsequent poll confirms consumed tokens (replay prevention)
	pollConsumed, err := mgr.PollLoopback(ctx, initRes.State)
	if err != nil || pollConsumed.Status != cliauth.StatusConsumed {
		t.Fatalf("expected consumed status on re-poll, got: %+v, err: %v", pollConsumed, err)
	}
	if pollConsumed.AccessToken != "" {
		t.Fatal("consumed session must not return tokens")
	}
}
