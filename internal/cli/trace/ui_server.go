// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// TraceUIServer represents a running local Trace UI dashboard server.
type TraceUIServer struct {
	URL    string
	Port   int
	server *http.Server
	mu     sync.Mutex
}

// Close gracefully stops the Trace UI server.
func (s *TraceUIServer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.Shutdown(ctx)
}

// Shutdown gracefully stops the Trace UI server with context.
func (s *TraceUIServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// isAllowedHost verifies that the Host header is local to prevent DNS rebinding attacks.
func isAllowedHost(hostHeader string, boundPort int) bool {
	if hostHeader == "" {
		return false
	}

	host, portStr, err := net.SplitHostPort(hostHeader)
	if err != nil {
		host = hostHeader
		portStr = ""
	}

	host = strings.ToLower(strings.TrimSpace(host))
	if host != "localhost" && host != "127.0.0.1" && host != "::1" && host != "[::1]" {
		return false
	}

	if portStr != "" && boundPort > 0 {
		if p, err := strconv.Atoi(portStr); err == nil && p != boundPort {
			return false
		}
	}

	return true
}

// StartTraceUIServer starts an HTTP server serving the Trace UI and local JSON APIs.
func StartTraceUIServer(gitRoot string, port int, host string) (*TraceUIServer, error) {
	if host == "" {
		host = "127.0.0.1"
	}

	var listener net.Listener
	var err error
	if port == 0 {
		listener, err = net.Listen("tcp", fmt.Sprintf("%s:0", host))
		if err != nil {
			return nil, fmt.Errorf("failed binding trace UI listener: %w", err)
		}
	} else {
		if port < 0 {
			port = 4711
		}
		listener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			// Fallback to dynamic port if specified port is occupied
			listener, err = net.Listen("tcp", fmt.Sprintf("%s:0", host))
			if err != nil {
				return nil, fmt.Errorf("failed binding trace UI listener: %w", err)
			}
		}
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()

	// Decision index cache
	loadDecisions := func(sessionID string) []TraceDecision {
		records, _ := ReadAllBranchRecords(context.Background(), gitRoot, "origin")
		var matched []TraceDecision
		for _, r := range records {
			for _, d := range r.Decisions {
				for _, sid := range d.SessionIDs {
					if sid == sessionID {
						matched = append(matched, d)
						break
					}
				}
			}
		}
		return matched
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !isAllowedHost(r.Host, actualPort) {
			http.Error(w, `{"error":"Untrusted Host header"}`, http.StatusMisdirectedRequest)
			return
		}

		path := r.URL.Path
		if path == "" || path == "/" || path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(RenderTraceUIHTML()))
			return
		}

		if path == "/api/sessions" {
			sessions, err := ListSessions(gitRoot)
			if err != nil {
				sessions = []TraceSessionSummary{}
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions": sessions,
			})
			return
		}

		if strings.HasPrefix(path, "/api/sessions/") {
			rawID := strings.TrimPrefix(path, "/api/sessions/")
			sessionID, err := url.PathUnescape(rawID)
			if err != nil {
				sessionID = rawID
			}

			session, err := ReadSession(gitRoot, sessionID)
			if err != nil || session == nil {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"session":   nil,
					"decisions": []TraceDecision{},
				})
				return
			}

			decisions := loadDecisions(sessionID)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session":   session,
				"decisions": decisions,
			})
			return
		}

		http.NotFound(w, r)
	})

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	traceServer := &TraceUIServer{
		URL:    fmt.Sprintf("http://%s:%d", host, actualPort),
		Port:   actualPort,
		server: server,
	}

	go func() {
		_ = server.Serve(listener)
	}()

	return traceServer, nil
}

// LaunchTraceUI starts the Trace UI server and blocks until SIGINT or SIGTERM.
func LaunchTraceUI(port int) error {
	server, err := StartTraceUIServer(".", port, "127.0.0.1")
	if err != nil {
		return err
	}
	defer server.Close()

	fmt.Printf("✔ ScanDrix Trace UI running at %s\n", server.URL)
	fmt.Println("  Reads the local store only. Press Ctrl-C to stop.")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("\nStopping Trace UI server...")
	return nil
}
