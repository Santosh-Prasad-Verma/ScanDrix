// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// SECURITY MANAGER
// =============================================================================

type SecurityManager struct {
	policy *SecurityPolicy
	tenant *TenantContext
}

func NewSecurityManager(policy *SecurityPolicy, tenant *TenantContext) *SecurityManager {
	return &SecurityManager{
		policy: policy,
		tenant: tenant,
	}
}

func (sm *SecurityManager) ValidateFileAccess(uri string) error {
	if sm.policy == nil {
		return nil
	}

	// Path traversal protection
	if sm.policy.PreventPathTraversal {
		if strings.Contains(uri, "..") || strings.Contains(uri, "~") {
			return fmt.Errorf("path traversal sequence detected in URI: %s", uri)
		}
	}

	// Check against allowed patterns
	if len(sm.policy.AllowedURIPatterns) > 0 {
		allowed := false
		for _, pattern := range sm.policy.AllowedURIPatterns {
			if pattern.MatchString(uri) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("URI not allowed by security policy: %s", uri)
		}
	}

	// Check against blocked patterns
	for _, pattern := range sm.policy.BlockedURIPatterns {
		if pattern.MatchString(uri) {
			return fmt.Errorf("URI blocked by security policy: %s", uri)
		}
	}

	// Tenant-specific roots validation
	if sm.tenant != nil && len(sm.tenant.AllowedRoots) > 0 {
		rootAllowed := false
		for _, root := range sm.tenant.AllowedRoots {
			if strings.HasPrefix(uri, root) {
				rootAllowed = true
				break
			}
		}
		if !rootAllowed {
			return fmt.Errorf("URI '%s' is outside tenant allowed roots: %v", uri, sm.tenant.AllowedRoots)
		}
	}

	return nil
}

func (sm *SecurityManager) CheckPermission(action string) bool {
	if sm.tenant == nil || len(sm.tenant.Permissions) == 0 {
		return true // default permissive if no explicit permissions array
	}

	for _, p := range sm.tenant.Permissions {
		if p == "*" || p == action {
			return true
		}
		if strings.HasSuffix(p, ":*") && strings.HasPrefix(action, strings.TrimSuffix(p, "*")) {
			return true
		}
	}
	return false
}

// =============================================================================
// CONSENT MANAGER (Prevents Confused Deputy Problem)
// =============================================================================

type ConsentRecord struct {
	ClientID    string `json:"clientId"`
	RedirectURI string `json:"redirectUri"`
	GrantedAt   int64  `json:"grantedAt"`
	ExpiresAt   int64  `json:"expiresAt"`
	TenantID    string `json:"tenantId"`
	UserID      string `json:"userId,omitempty"`
}

type ConsentManager struct {
	mu       sync.RWMutex
	consents map[string]*ConsentRecord
}

func NewConsentManager() *ConsentManager {
	return &ConsentManager{
		consents: make(map[string]*ConsentRecord),
	}
}

func (cm *ConsentManager) RequiresAdditionalConsent(clientID, tenantID string) bool {
	key := tenantID + ":" + clientID
	cm.mu.RLock()
	record, exists := cm.consents[key]
	cm.mu.RUnlock()

	if !exists {
		return true
	}

	if time.Now().UnixMilli() > record.ExpiresAt {
		cm.mu.Lock()
		delete(cm.consents, key)
		cm.mu.Unlock()
		return true
	}

	return false
}

func (cm *ConsentManager) GrantConsent(clientID, redirectURI, tenantID, userID string) {
	key := tenantID + ":" + clientID
	now := time.Now().UnixMilli()

	record := &ConsentRecord{
		ClientID:    clientID,
		RedirectURI: redirectURI,
		GrantedAt:   now,
		ExpiresAt:   now + (24 * time.Hour).Milliseconds(),
		TenantID:    tenantID,
		UserID:      userID,
	}

	cm.mu.Lock()
	cm.consents[key] = record
	cm.mu.Unlock()
}

func (cm *ConsentManager) ValidateConsent(clientID, redirectURI, tenantID string) bool {
	key := tenantID + ":" + clientID
	cm.mu.RLock()
	record, exists := cm.consents[key]
	cm.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().UnixMilli() > record.ExpiresAt {
		cm.mu.Lock()
		delete(cm.consents, key)
		cm.mu.Unlock()
		return false
	}

	return record.RedirectURI == redirectURI
}

// =============================================================================
// TOKEN VALIDATOR (Prevents Token Passthrough)
// =============================================================================

type TokenValidator struct {
	expectedAudience string
	jwtValidator     *JWTValidator
}

func NewTokenValidator(serverURL, tenantID string) *TokenValidator {
	cleanURL := strings.TrimPrefix(strings.TrimPrefix(serverURL, "http://"), "https://")
	expectedAud := fmt.Sprintf("mcp://%s/%s", cleanURL, tenantID)

	return &TokenValidator{
		expectedAudience: expectedAud,
		jwtValidator:     ForServiceTokens(expectedAud),
	}
}

func (tv *TokenValidator) ValidateTokenAudience(token string) bool {
	return tv.jwtValidator.ValidateAudience(token, tv.expectedAudience)
}

func (tv *TokenValidator) ValidateTokenFormat(token string) bool {
	return tv.jwtValidator.IsValidJWTFormat(token)
}

func (tv *TokenValidator) IsTokenExpired(token string) bool {
	return tv.jwtValidator.IsExpired(token)
}

// =============================================================================
// METRICS COLLECTOR & AUDIT LOGGER
// =============================================================================

type MetricsCollector struct {
	mu       sync.Mutex
	tenantID string
	metrics  MCPMetrics
}

func NewMetricsCollector(tenantID string) *MetricsCollector {
	return &MetricsCollector{
		tenantID: tenantID,
		metrics: MCPMetrics{
			RequestDuration: make([]float64, 0, 100),
			TenantMetrics:   make(map[string]*TenantMetric),
		},
	}
}

func (mc *MetricsCollector) RecordConnection(success bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.metrics.ConnectionsTotal++
	if success {
		mc.metrics.ConnectionsActive++
	} else {
		mc.metrics.ConnectionErrors++
	}
}

func (mc *MetricsCollector) RecordRequest(reqType string, duration float64, success bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.metrics.RequestsTotal++
	mc.metrics.RequestDuration = append(mc.metrics.RequestDuration, duration)
	if len(mc.metrics.RequestDuration) > 100 {
		mc.metrics.RequestDuration = mc.metrics.RequestDuration[len(mc.metrics.RequestDuration)-100:]
	}

	if success {
		mc.metrics.RequestsSuccessful++
	} else {
		mc.metrics.RequestsFailed++
	}

	switch reqType {
	case "tool_call":
		mc.metrics.ToolCalls++
	case "resource_read":
		mc.metrics.ResourceReads++
	case "prompt_get":
		mc.metrics.PromptGets++
	case "sampling":
		mc.metrics.SamplingRequests++
	case "elicitation":
		mc.metrics.ElicitationRequests++
	}

	tm, exists := mc.metrics.TenantMetrics[mc.tenantID]
	if !exists {
		tm = &TenantMetric{}
		mc.metrics.TenantMetrics[mc.tenantID] = tm
	}
	tm.Requests++
	if !success {
		tm.Errors++
	}
}

func (mc *MetricsCollector) RecordSecurityEvent(eventType string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	switch eventType {
	case "violation":
		mc.metrics.SecurityViolations++
	case "unauthorized":
		mc.metrics.UnauthorizedAccess++
	case "path_traversal":
		mc.metrics.PathTraversalAttempts++
	}
}

func (mc *MetricsCollector) GetMetrics() MCPMetrics {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	copyDur := make([]float64, len(mc.metrics.RequestDuration))
	copy(copyDur, mc.metrics.RequestDuration)

	copyTM := make(map[string]*TenantMetric, len(mc.metrics.TenantMetrics))
	for k, v := range mc.metrics.TenantMetrics {
		copyTM[k] = &TenantMetric{
			Requests:   v.Requests,
			TokensUsed: v.TokensUsed,
			Errors:     v.Errors,
		}
	}

	res := mc.metrics
	res.RequestDuration = copyDur
	res.TenantMetrics = copyTM
	return res
}

type AuditLogger struct {
	mu       sync.RWMutex
	tenantID string
	events   []AuditEvent
}

func NewAuditLogger(tenantID string) *AuditLogger {
	return &AuditLogger{
		tenantID: tenantID,
		events:   make([]AuditEvent, 0, 100),
	}
}

func (al *AuditLogger) Log(event AuditEvent) {
	al.mu.Lock()
	defer al.mu.Unlock()

	if event.Timestamp == 0 {
		event.Timestamp = time.Now().UnixMilli()
	}
	if event.TenantID == "" {
		event.TenantID = al.tenantID
	}

	if len(al.events) >= 100 {
		al.events = al.events[1:]
	}
	al.events = append(al.events, event)
}

func (al *AuditLogger) GetEvents() []AuditEvent {
	al.mu.RLock()
	defer al.mu.RUnlock()
	copyEvents := make([]AuditEvent, len(al.events))
	copy(copyEvents, al.events)
	return copyEvents
}

// =============================================================================
// SPEC-COMPLIANT MCP CLIENT
// =============================================================================

// SpecCompliantMCPClient provides complete client-side connectivity to upstream MCP servers.
type SpecCompliantMCPClient struct {
	config           MCPClientConfig
	httpClient       *http.Client
	connected        bool
	mu               sync.RWMutex
	securityManager  *SecurityManager
	consentManager   *ConsentManager
	tokenValidator   *TokenValidator
	sessionManager   *SessionManager
	metricsCollector *MetricsCollector
	auditLogger      *AuditLogger
	approvalHandler  HumanApprovalHandler
	cachedRoots      []string
}

// NewSpecCompliantMCPClient instantiates a new MCP client.
func NewSpecCompliantMCPClient(config MCPClientConfig) *SpecCompliantMCPClient {
	tenantID := "default"
	if config.Tenant != nil && config.Tenant.TenantID != "" {
		tenantID = config.Tenant.TenantID
	}

	timeout := config.Transport.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	c := &SpecCompliantMCPClient{
		config: config,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		consentManager:   NewConsentManager(),
		sessionManager:   NewSessionManager(),
		metricsCollector: NewMetricsCollector(tenantID),
		auditLogger:      NewAuditLogger(tenantID),
	}

	if config.Security != nil && config.Tenant != nil {
		c.securityManager = NewSecurityManager(config.Security, config.Tenant)
	}

	if config.Transport.URL != "" {
		c.tokenValidator = NewTokenValidator(config.Transport.URL, tenantID)
	}

	return c
}

func (c *SpecCompliantMCPClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return errors.New("already connected")
	}

	startTime := time.Now()

	if c.securityManager != nil && !c.securityManager.CheckPermission("connect") {
		c.metricsCollector.RecordSecurityEvent("unauthorized")
		return errors.New("permission denied: connect")
	}

	// Validate auth headers if present
	if err := c.validateAuthHeaders(); err != nil {
		c.metricsCollector.RecordSecurityEvent("violation")
		return err
	}

	// Handle transport initialization
	switch c.config.Transport.Type {
	case TransportHTTP, TransportSSE:
		if c.config.Transport.URL == "" {
			return errors.New("URL is required for HTTP/SSE transport")
		}
		// Send handshake initialize request
		initReq := map[string]any{
			"jsonrpc": "2.0",
			"id":      uuid.NewString(),
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
				"clientInfo":      c.config.ClientInfo,
				"capabilities":    c.config.Capabilities,
			},
		}

		resp, err := c.executeJSONRPC(ctx, initReq)
		if err != nil {
			c.metricsCollector.RecordConnection(false)
			c.auditLogger.Log(AuditEvent{
				Event:   "connection_failed",
				Success: false,
				Error:   err.Error(),
			})
			return fmt.Errorf("handshake failed with MCP server: %w", err)
		}
		_ = resp

	case TransportStdio:
		if c.config.Transport.Command == "" {
			return errors.New("command is required for stdio transport")
		}

	default:
		return fmt.Errorf("unsupported transport: %s", c.config.Transport.Type)
	}

	c.connected = true
	c.metricsCollector.RecordConnection(true)
	c.auditLogger.Log(AuditEvent{
		Event:   "connection_established",
		Success: true,
		Metadata: map[string]any{
			"durationMs": time.Since(startTime).Milliseconds(),
		},
	})

	return nil
}

func (c *SpecCompliantMCPClient) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false
	c.sessionManager.Destroy()
	c.cachedRoots = nil

	c.auditLogger.Log(AuditEvent{
		Event:   "connection_closed",
		Success: true,
	})

	return nil
}

func (c *SpecCompliantMCPClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *SpecCompliantMCPClient) ListTools(ctx context.Context) ([]*MCPToolRaw, error) {
	if !c.IsConnected() {
		return nil, errors.New("not connected to MCP server")
	}

	if c.securityManager != nil && !c.securityManager.CheckPermission("tools.list") {
		c.metricsCollector.RecordSecurityEvent("unauthorized")
		return nil, errors.New("permission denied: tools.list")
	}

	startTime := time.Now()
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "tools/list",
		"params":  map[string]any{},
	}

	resp, err := c.executeJSONRPC(ctx, req)
	dur := float64(time.Since(startTime).Milliseconds())
	if err != nil {
		c.metricsCollector.RecordRequest("tools_list", dur, false)
		return nil, err
	}
	c.metricsCollector.RecordRequest("tools_list", dur, true)

	rawTools, ok := resp["tools"].([]any)
	if !ok {
		return []*MCPToolRaw{}, nil
	}

	var tools []*MCPToolRaw
	for _, item := range rawTools {
		itemMap, isMap := item.(map[string]any)
		if !isMap {
			continue
		}

		name, _ := itemMap["name"].(string)
		if name == "" {
			continue
		}

		// Apply allowed tools filter if specified
		if len(c.config.AllowedTools) > 0 {
			allowed := false
			for _, allowedName := range c.config.AllowedTools {
				if allowedName == name {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		inputSchema, _ := itemMap["inputSchema"].(map[string]any)
		if inputSchema == nil {
			inputSchema = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}

		outputSchema, _ := itemMap["outputSchema"].(map[string]any)
		annotations, _ := itemMap["annotations"].(map[string]any)
		desc, _ := itemMap["description"].(string)
		title, _ := itemMap["title"].(string)

		tools = append(tools, &MCPToolRaw{
			Name:         name,
			Title:        title,
			Description:  desc,
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
			Annotations:  annotations,
		})
	}

	return tools, nil
}

func (c *SpecCompliantMCPClient) ExecuteTool(ctx context.Context, name string, args map[string]any) (any, error) {
	if !c.IsConnected() {
		return nil, errors.New("not connected to MCP server")
	}

	if c.securityManager != nil && !c.securityManager.CheckPermission("tools.call") {
		c.metricsCollector.RecordSecurityEvent("unauthorized")
		return nil, errors.New("permission denied: tools.call")
	}

	maxRetries := c.config.Transport.Retries
	if maxRetries < 1 {
		maxRetries = 1
	}

	var lastErr error
	startTime := time.Now()

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req := map[string]any{
			"jsonrpc": "2.0",
			"id":      uuid.NewString(),
			"method":  "tools/call",
			"params": map[string]any{
				"name":      name,
				"arguments": args,
			},
		}

		resp, err := c.executeJSONRPC(ctx, req)
		if err == nil {
			dur := float64(time.Since(startTime).Milliseconds())
			c.metricsCollector.RecordRequest("tool_call", dur, true)
			c.auditLogger.Log(AuditEvent{
				Event:    "tool_called",
				Resource: name,
				Success:  true,
			})

			// Check for protocol-level isError flag in response
			if isErr, _ := resp["isError"].(bool); isErr {
				errMsg := "tool execution reported failure"
				if content, ok := resp["content"].([]any); ok && len(content) > 0 {
					var textParts []string
					for _, c := range content {
						if cm, ok := c.(map[string]any); ok {
							if txt, hasTxt := cm["text"].(string); hasTxt {
								textParts = append(textParts, txt)
							}
						}
					}
					if len(textParts) > 0 {
						errMsg = strings.Join(textParts, "\n")
					}
				}
				return nil, fmt.Errorf("tool %s failed: %s", name, errMsg)
			}

			return resp, nil
		}

		lastErr = err
		dur := float64(time.Since(startTime).Milliseconds())
		c.metricsCollector.RecordRequest("tool_call", dur, false)

		// If retryable error, backoff with jitter
		if attempt < maxRetries && c.isRetryableError(err) {
			delay := c.calculateRetryDelay(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		} else {
			break
		}
	}

	c.auditLogger.Log(AuditEvent{
		Event:    "tool_call_failed",
		Resource: name,
		Success:  false,
		Error:    lastErr.Error(),
	})

	return nil, lastErr
}

func (c *SpecCompliantMCPClient) ListResources(ctx context.Context) ([]MCPResource, error) {
	if !c.IsConnected() {
		return nil, errors.New("not connected to MCP server")
	}

	if c.securityManager != nil && !c.securityManager.CheckPermission("resources.list") {
		c.metricsCollector.RecordSecurityEvent("unauthorized")
		return nil, errors.New("permission denied: resources.list")
	}

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "resources/list",
		"params":  map[string]any{},
	}

	resp, err := c.executeJSONRPC(ctx, req)
	if err != nil {
		return nil, err
	}

	rawRes, _ := resp["resources"].([]any)
	var resources []MCPResource
	for _, item := range rawRes {
		if m, ok := item.(map[string]any); ok {
			uri, _ := m["uri"].(string)
			name, _ := m["name"].(string)
			desc, _ := m["description"].(string)
			mime, _ := m["mimeType"].(string)
			resources = append(resources, MCPResource{
				URI:         uri,
				Name:        name,
				Description: desc,
				MimeType:    mime,
			})
		}
	}

	return resources, nil
}

func (c *SpecCompliantMCPClient) ReadResource(ctx context.Context, uri string) (any, error) {
	if !c.IsConnected() {
		return nil, errors.New("not connected to MCP server")
	}

	if c.securityManager != nil {
		if !c.securityManager.CheckPermission("resources.read") {
			c.metricsCollector.RecordSecurityEvent("unauthorized")
			return nil, errors.New("permission denied: resources.read")
		}
		if err := c.securityManager.ValidateFileAccess(uri); err != nil {
			if strings.Contains(err.Error(), "path traversal") {
				c.metricsCollector.RecordSecurityEvent("path_traversal")
			} else {
				c.metricsCollector.RecordSecurityEvent("unauthorized")
			}
			return nil, err
		}
	}

	startTime := time.Now()
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "resources/read",
		"params": map[string]any{
			"uri": uri,
		},
	}

	resp, err := c.executeJSONRPC(ctx, req)
	dur := float64(time.Since(startTime).Milliseconds())
	if err != nil {
		c.metricsCollector.RecordRequest("resource_read", dur, false)
		return nil, err
	}

	c.metricsCollector.RecordRequest("resource_read", dur, true)
	c.auditLogger.Log(AuditEvent{
		Event:    "resource_read",
		Resource: uri,
		Success:  true,
	})

	return resp, nil
}

func (c *SpecCompliantMCPClient) ListPrompts(ctx context.Context) ([]MCPPrompt, error) {
	if !c.IsConnected() {
		return nil, errors.New("not connected to MCP server")
	}

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "prompts/list",
		"params":  map[string]any{},
	}

	resp, err := c.executeJSONRPC(ctx, req)
	if err != nil {
		return nil, err
	}

	rawPrompts, _ := resp["prompts"].([]any)
	var prompts []MCPPrompt
	for _, item := range rawPrompts {
		if m, ok := item.(map[string]any); ok {
			name, _ := m["name"].(string)
			desc, _ := m["description"].(string)

			var args []PromptArgument
			if rawArgs, ok := m["arguments"].([]any); ok {
				for _, ra := range rawArgs {
					if am, ok := ra.(map[string]any); ok {
						an, _ := am["name"].(string)
						ad, _ := am["description"].(string)
						ar, _ := am["required"].(bool)
						args = append(args, PromptArgument{
							Name:        an,
							Description: ad,
							Required:    ar,
						})
					}
				}
			}

			prompts = append(prompts, MCPPrompt{
				Name:        name,
				Description: desc,
				Arguments:   args,
			})
		}
	}

	return prompts, nil
}

func (c *SpecCompliantMCPClient) GetPrompt(ctx context.Context, name string, args map[string]string) (any, error) {
	if !c.IsConnected() {
		return nil, errors.New("not connected to MCP server")
	}

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "prompts/get",
		"params": map[string]any{
			"name":      name,
			"arguments": args,
		},
	}

	return c.executeJSONRPC(ctx, req)
}

func (c *SpecCompliantMCPClient) CreateMessage(ctx context.Context, req CreateMessageRequest) (*CreateMessageResult, error) {
	if !c.IsConnected() {
		return nil, errors.New("not connected to MCP server")
	}

	if c.securityManager != nil && !c.securityManager.CheckPermission("sampling:create") {
		return nil, errors.New("permission denied: sampling:create")
	}

	// Human-in-the-loop oversight
	if c.approvalHandler != nil {
		risk := "low"
		if req.MaxTokens > 1000 {
			risk = "medium"
		}
		appResp, err := c.approvalHandler.RequestApproval(ctx, HumanApprovalRequest{
			Type:    "sampling",
			Message: fmt.Sprintf("Request to create LLM message with %d messages", len(req.Messages)),
			Context: ApprovalContext{
				Server: "upstream",
				Action: "sampling/createMessage",
				Security: &ApprovalSecurityContext{
					RiskLevel: risk,
					Reason:    "LLM sampling requires human oversight",
				},
			},
			Timeout: 30 * time.Second,
		})
		if err != nil {
			return nil, fmt.Errorf("approval request failed: %w", err)
		}
		if !appResp.Approved {
			return nil, fmt.Errorf("sampling request denied: %s", appResp.Reason)
		}
	}

	jsonReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "sampling/createMessage",
		"params":  req,
	}

	resp, err := c.executeJSONRPC(ctx, jsonReq)
	if err != nil {
		return nil, err
	}

	model, _ := resp["model"].(string)
	content, _ := resp["content"].(string)
	role, _ := resp["role"].(string)

	return &CreateMessageResult{
		Model:   model,
		Content: content,
		Role:    role,
	}, nil
}

func (c *SpecCompliantMCPClient) CreateElicitation(ctx context.Context, req CreateElicitationRequest) (*CreateElicitationResult, error) {
	if !c.IsConnected() {
		return nil, errors.New("not connected to MCP server")
	}

	if c.securityManager != nil && !c.securityManager.CheckPermission("elicitation:create") {
		return nil, errors.New("permission denied: elicitation:create")
	}

	if c.approvalHandler != nil {
		appResp, err := c.approvalHandler.RequestApproval(ctx, HumanApprovalRequest{
			Type:    "elicitation",
			Message: req.Message,
			Context: ApprovalContext{
				Server: "upstream",
				Action: "elicitation/create",
				Security: &ApprovalSecurityContext{
					RiskLevel: "medium",
					Reason:    "User information elicitation request",
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("approval error: %w", err)
		}
		if !appResp.Approved {
			return nil, fmt.Errorf("elicitation request denied: %s", appResp.Reason)
		}
	}

	jsonReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "elicitation/create",
		"params":  req,
	}

	resp, err := c.executeJSONRPC(ctx, jsonReq)
	if err != nil {
		return nil, err
	}

	action, _ := resp["action"].(string)
	msg, _ := resp["message"].(string)

	return &CreateElicitationResult{
		Action:  action,
		Data:    resp["data"],
		Message: msg,
	}, nil
}

func (c *SpecCompliantMCPClient) Ping(ctx context.Context) error {
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "ping",
	}
	_, err := c.executeJSONRPC(ctx, req)
	return err
}

func (c *SpecCompliantMCPClient) SetApprovalHandler(h HumanApprovalHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.approvalHandler = h
}

func (c *SpecCompliantMCPClient) GetMetrics() MCPMetrics {
	return c.metricsCollector.GetMetrics()
}

func (c *SpecCompliantMCPClient) GetAuditEvents() []AuditEvent {
	return c.auditLogger.GetEvents()
}

// ─── Helpers ─────────────────────────────────────────────────────────────

func (c *SpecCompliantMCPClient) executeJSONRPC(ctx context.Context, payload map[string]any) (map[string]any, error) {
	switch c.config.Transport.Type {
	case TransportHTTP, TransportSSE:
		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.Transport.URL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json, text/event-stream")

		for k, v := range c.config.Transport.Headers {
			httpReq.Header.Set(k, v)
		}

		httpResp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return nil, err
		}
		defer httpResp.Body.Close()

		if httpResp.StatusCode >= 400 {
			errBytes, _ := io.ReadAll(httpResp.Body)
			return nil, fmt.Errorf("server returned error %d: %s", httpResp.StatusCode, string(errBytes))
		}

		respBytes, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return nil, err
		}

		var rpcResp struct {
			JSONRPC string         `json:"jsonrpc"`
			ID      any            `json:"id"`
			Result  map[string]any `json:"result"`
			Error   *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Data    any    `json:"data"`
			} `json:"error"`
		}

		if err := json.Unmarshal(respBytes, &rpcResp); err != nil {
			return nil, fmt.Errorf("failed to parse JSON-RPC response: %w", err)
		}

		if rpcResp.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
		}

		if rpcResp.Result == nil {
			return make(map[string]any), nil
		}

		return rpcResp.Result, nil

	case TransportStdio:
		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		cmd := exec.CommandContext(ctx, c.config.Transport.Command, c.config.Transport.Args...)
		cmd.Stdin = bytes.NewReader(bodyBytes)
		if c.config.Transport.Cwd != "" {
			cmd.Dir = c.config.Transport.Cwd
		}
		if len(c.config.Transport.Env) > 0 {
			cmd.Env = os.Environ()
			for k, v := range c.config.Transport.Env {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}
		}

		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf

		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("stdio command execution failed: %w (%s)", err, errBuf.String())
		}

		var rpcResp struct {
			Result map[string]any `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(outBuf.Bytes(), &rpcResp); err != nil {
			return nil, fmt.Errorf("failed to parse stdio JSON-RPC response: %w", err)
		}

		if rpcResp.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
		}

		return rpcResp.Result, nil

	default:
		return nil, fmt.Errorf("unsupported transport type: %s", c.config.Transport.Type)
	}
}

func (c *SpecCompliantMCPClient) validateAuthHeaders() error {
	headers := c.config.Transport.Headers
	if headers == nil || c.tokenValidator == nil {
		return nil
	}

	authHeader := headers["Authorization"]
	if authHeader == "" {
		authHeader = headers["authorization"]
	}
	if authHeader == "" {
		return nil
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")

	if !c.tokenValidator.ValidateTokenFormat(token) {
		return errors.New("invalid authorization token format")
	}

	if !c.tokenValidator.ValidateTokenAudience(token) {
		return errors.New("token was not issued for this MCP server (audience mismatch)")
	}

	if c.tokenValidator.IsTokenExpired(token) {
		return errors.New("authorization token has expired")
	}

	return nil
}

func (c *SpecCompliantMCPClient) isRetryableError(err error) bool {
	msg := strings.ToLower(err.Error())
	retryablePatterns := []string{
		"timeout", "network", "connection", "temporary", "rate limit",
		"too many requests", "service unavailable", "reset by peer",
	}
	for _, p := range retryablePatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

func (c *SpecCompliantMCPClient) calculateRetryDelay(attempt int) time.Duration {
	baseDelay := 1000 * time.Millisecond
	maxDelay := 10 * time.Second

	exponential := baseDelay * time.Duration(1<<(attempt-1))
	// Add jitter +-25%
	jitterFrac := (rand.Float64()*0.5 - 0.25)
	jittered := float64(exponential) * (1.0 + jitterFrac)

	delay := time.Duration(jittered)
	if delay > maxDelay {
		delay = maxDelay
	}
	if delay < 100*time.Millisecond {
		delay = 100 * time.Millisecond
	}
	return delay
}
