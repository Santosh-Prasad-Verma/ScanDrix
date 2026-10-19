package threatmodel

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// StrideCategory represents the STRIDE threat classification.
type StrideCategory string

const (
	StrideSpoofing             StrideCategory = "Spoofing Identity"
	StrideTampering            StrideCategory = "Tampering with Data"
	StrideRepudiation          StrideCategory = "Repudiation"
	StrideInfoDisclosure       StrideCategory = "Information Disclosure"
	StrideDenialOfService      StrideCategory = "Denial of Service"
	StrideElevationOfPrivilege StrideCategory = "Elevation of Privilege"
)

// AttackSurfaceEntry describes a detected input ingress point or sensitive egress sink.
type AttackSurfaceEntry struct {
	ID              string         `json:"id"`
	Type            string         `json:"type"`             // HTTP_ENDPOINT, FILE_UPLOAD, WEBHOOK, CLI_ARG, SQL_SINK, EXEC_SINK, SENSITIVE_LOG_SINK
	Location        string         `json:"location"`         // path/file.go:line
	Handler         string         `json:"handler"`          // Function / Route name
	UntrustedInputs []string       `json:"untrusted_inputs"` // Parameters or query fields
	RiskCategory    StrideCategory `json:"risk_category"`    // STRIDE classification
	Severity        string         `json:"severity"`         // CRITICAL, HIGH, MEDIUM, LOW
	Mitigation      string         `json:"mitigation"`       // Enforced defensive control
}

// TrustBoundary defines a boundary crossing between different security zones.
type TrustBoundary struct {
	FromDomain string `json:"from_domain"`
	ToDomain   string `json:"to_domain"`
	Barrier    string `json:"barrier"`
}

// ThreatDAGNode represents a vertex in the attack surface Directed Acyclic Graph.
type ThreatDAGNode struct {
	ID           string         `json:"id"`
	NodeType     string         `json:"node_type"` // INGRESS, TRANSFORM, SINK
	Label        string         `json:"label"`
	Location     string         `json:"location"`
	RiskCategory StrideCategory `json:"risk_category"`
	Severity     string         `json:"severity"`
}

// ThreatDAGEdge represents a data flow transition between ingress points and sinks.
type ThreatDAGEdge struct {
	SourceID    string `json:"source_id"`
	TargetID    string `json:"target_id"`
	DataFlow    string `json:"data_flow"`
	TaintStatus string `json:"taint_status"` // TAINTED_UNVALIDATED, SANITIZED
}

// ThreatDAG represents the complete Directed Acyclic Graph of attack surface data flows.
type ThreatDAG struct {
	Nodes         []ThreatDAGNode `json:"nodes"`
	Edges         []ThreatDAGEdge `json:"edges"`
	EntryPoints   []string        `json:"entry_points"`
	HighRiskPaths int             `json:"high_risk_paths"`
}

// ThreatModelReport summarizes the security topology and attack surface of code changes.
type ThreatModelReport struct {
	Overview        string               `json:"overview"`
	AttackSurface   []AttackSurfaceEntry `json:"attack_surface"`
	TrustBoundaries []TrustBoundary      `json:"trust_boundaries"`
	DAG             *ThreatDAG           `json:"dag,omitempty"`
	GeneratedAt     time.Time            `json:"generated_at"`
}

var (
	routePattern   = regexp.MustCompile(`(?i)(r\.(Get|Post|Put|Delete|Patch)|http\.HandleFunc|app\.(get|post|put|delete))\s*\(\s*["']([^"']+)["']`)
	uploadPattern  = regexp.MustCompile(`(?i)(r\.(MultipartReader|FormFile)|multer|UploadFile|file\.SaveAs|upload_to)`)
	webhookPattern = regexp.MustCompile(`(?i)(webhook|handleWebhook|WebhookPayload|VerifySignature|x-hub-signature)`)
	cliPattern     = regexp.MustCompile(`(?i)(flag\.(String|Int|Bool|StringVar)|cobra\.Command|os\.Args|clap|argparse)`)
	sqlPattern     = regexp.MustCompile(`(?i)(db\.Query|db\.Exec|pool\.Query|pgx\.|SELECT|INSERT|UPDATE|DELETE).*\+.*`)
	execPattern    = regexp.MustCompile(`(?i)(exec\.Command|os\.StartProcess|child_process|system\()`)
	authPattern    = regexp.MustCompile(`(?i)(jwt|bearer|password|token|api_key|secret)`)
)

// AnalyzeFileThreats scans a source file and extracts entry points, sinks, and STRIDE threats.
func AnalyzeFileThreats(filePath, content string) []AttackSurfaceEntry {
	var entries []AttackSurfaceEntry
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Ignore comment lines
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// 1. Ingress HTTP Endpoints (Elevation of Privilege / Spoofing)
		if m := routePattern.FindStringSubmatch(trimmed); len(m) > 4 {
			route := m[4]
			entries = append(entries, AttackSurfaceEntry{
				ID:              fmt.Sprintf("entry-http-%d", lineNum),
				Type:            "HTTP_ENDPOINT",
				Location:        fmt.Sprintf("%s:%d", filePath, lineNum),
				Handler:         route,
				UntrustedInputs: []string{"req.Body", "req.URL.Query", "headers"},
				RiskCategory:    StrideElevationOfPrivilege,
				Severity:        "HIGH",
				Mitigation:      "Ensure authentication middleware and RBAC policy guards are declared.",
			})
		}

		// 2. Ingress File Upload Handlers (Tampering / Denial of Service)
		if uploadPattern.MatchString(trimmed) {
			entries = append(entries, AttackSurfaceEntry{
				ID:              fmt.Sprintf("entry-upload-%d", lineNum),
				Type:            "FILE_UPLOAD",
				Location:        fmt.Sprintf("%s:%d", filePath, lineNum),
				Handler:         trimmed,
				UntrustedInputs: []string{"multipart.File", "content-type", "file name"},
				RiskCategory:    StrideTampering,
				Severity:        "HIGH",
				Mitigation:      "Validate file magic bytes, enforce max size limits, and isolate stored files.",
			})
		}

		// 3. Ingress Webhook Receivers (Spoofing / Repudiation)
		if webhookPattern.MatchString(trimmed) {
			entries = append(entries, AttackSurfaceEntry{
				ID:              fmt.Sprintf("entry-webhook-%d", lineNum),
				Type:            "WEBHOOK_RECEIVER",
				Location:        fmt.Sprintf("%s:%d", filePath, lineNum),
				Handler:         trimmed,
				UntrustedInputs: []string{"webhook headers", "signature token", "event payload"},
				RiskCategory:    StrideSpoofing,
				Severity:        "HIGH",
				Mitigation:      "Verify cryptographic HMAC payload signature before processing event.",
			})
		}

		// 4. CLI Argument Ingress (Elevation of Privilege)
		if cliPattern.MatchString(trimmed) {
			entries = append(entries, AttackSurfaceEntry{
				ID:              fmt.Sprintf("entry-cli-%d", lineNum),
				Type:            "CLI_ARG",
				Location:        fmt.Sprintf("%s:%d", filePath, lineNum),
				Handler:         trimmed,
				UntrustedInputs: []string{"os.Args", "command-line flags"},
				RiskCategory:    StrideElevationOfPrivilege,
				Severity:        "MEDIUM",
				Mitigation:      "Strictly validate flags against typed whitelist schemas.",
			})
		}

		// 5. SQL Concatenation Sinks (Tampering / Info Disclosure)
		if sqlPattern.MatchString(trimmed) && strings.Contains(trimmed, "+") {
			entries = append(entries, AttackSurfaceEntry{
				ID:              fmt.Sprintf("sink-sql-%d", lineNum),
				Type:            "SQL_SINK",
				Location:        fmt.Sprintf("%s:%d", filePath, lineNum),
				Handler:         trimmed,
				UntrustedInputs: []string{"concatenated parameters"},
				RiskCategory:    StrideTampering,
				Severity:        "CRITICAL",
				Mitigation:      "Use parameterized queries with driver placeholders ($1, ?) to prevent SQL injection.",
			})
		}

		// 6. Shell Execution Sinks (Elevation of Privilege / Command Injection)
		if execPattern.MatchString(trimmed) {
			entries = append(entries, AttackSurfaceEntry{
				ID:              fmt.Sprintf("sink-exec-%d", lineNum),
				Type:            "EXEC_SINK",
				Location:        fmt.Sprintf("%s:%d", filePath, lineNum),
				Handler:         trimmed,
				UntrustedInputs: []string{"command arguments"},
				RiskCategory:    StrideElevationOfPrivilege,
				Severity:        "CRITICAL",
				Mitigation:      "Avoid invoking raw shell commands or sanitize arguments with strict allowlists.",
			})
		}

		// 7. Insecure Logging of Credentials (Information Disclosure)
		if strings.Contains(strings.ToLower(trimmed), "log") && authPattern.MatchString(trimmed) {
			entries = append(entries, AttackSurfaceEntry{
				ID:              fmt.Sprintf("sink-log-%d", lineNum),
				Type:            "SENSITIVE_LOG_SINK",
				Location:        fmt.Sprintf("%s:%d", filePath, lineNum),
				Handler:         trimmed,
				UntrustedInputs: []string{"credentials in log payload"},
				RiskCategory:    StrideInfoDisclosure,
				Severity:        "HIGH",
				Mitigation:      "Mask or omit tokens, passwords, and API keys from logging statements.",
			})
		}
	}

	return entries
}

// BuildThreatDAG constructs a Directed Acyclic Graph connecting untrusted ingress entry points to sinks.
func BuildThreatDAG(entries []AttackSurfaceEntry) *ThreatDAG {
	dag := &ThreatDAG{
		Nodes: make([]ThreatDAGNode, 0, len(entries)),
		Edges: make([]ThreatDAGEdge, 0),
	}

	var ingressNodes []ThreatDAGNode
	var sinkNodes []ThreatDAGNode

	for _, e := range entries {
		nodeType := "INGRESS"
		if strings.HasSuffix(e.Type, "_SINK") {
			nodeType = "SINK"
		}

		node := ThreatDAGNode{
			ID:           e.ID,
			NodeType:     nodeType,
			Label:        fmt.Sprintf("%s (%s)", e.Type, e.Location),
			Location:     e.Location,
			RiskCategory: e.RiskCategory,
			Severity:     e.Severity,
		}
		dag.Nodes = append(dag.Nodes, node)

		if nodeType == "INGRESS" {
			ingressNodes = append(ingressNodes, node)
			dag.EntryPoints = append(dag.EntryPoints, node.ID)
		} else {
			sinkNodes = append(sinkNodes, node)
		}
	}

	// Connect unvalidated data flows from ingress entrypoints to sensitive sinks
	for _, in := range ingressNodes {
		for _, sink := range sinkNodes {
			dag.Edges = append(dag.Edges, ThreatDAGEdge{
				SourceID:    in.ID,
				TargetID:    sink.ID,
				DataFlow:    fmt.Sprintf("Untrusted Ingress [%s] → Critical Sink [%s]", in.Label, sink.Label),
				TaintStatus: "TAINTED_UNVALIDATED",
			})
			dag.HighRiskPaths++
		}
	}

	return dag
}

// DeriveThreatModel aggregates threat analysis across modified files into a structured report with DAG.
func DeriveThreatModel(files map[string]string) *ThreatModelReport {
	report := &ThreatModelReport{
		GeneratedAt: time.Now().UTC(),
		TrustBoundaries: []TrustBoundary{
			{
				FromDomain: "Untrusted Client / SCM Webhook",
				ToDomain:   "ScanDrix API Gateway",
				Barrier:    "HMAC Webhook Signatures & Argon2id Session Tokens",
			},
			{
				FromDomain: "API Gateway",
				ToDomain:   "Multi-Tenant PostgreSQL Warehouse",
				Barrier:    "Least-Privilege Role & Row-Level Security (RLS) Tenant Isolation",
			},
			{
				FromDomain: "Worker Execution Harness",
				ToDomain:   "Ephemeral Sandbox Runner",
				Barrier:    "Docker Ephemeral Container & Network Egress Allowlist",
			},
		},
	}

	for path, code := range files {
		entries := AnalyzeFileThreats(path, code)
		report.AttackSurface = append(report.AttackSurface, entries...)
	}

	report.DAG = BuildThreatDAG(report.AttackSurface)
	report.Overview = fmt.Sprintf("Analyzed %d source file(s): identified %d attack surface entry point(s) and sensitive sink(s) across %d high-risk data-flow path(s).",
		len(files), len(report.AttackSurface), report.DAG.HighRiskPaths)

	return report
}
