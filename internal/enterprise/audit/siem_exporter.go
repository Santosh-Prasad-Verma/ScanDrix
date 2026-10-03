package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SIEMExportFormat defines the wire encoding for external security collectors.
type SIEMExportFormat string

const (
	ExportFormatCEF     SIEMExportFormat = "CEF"
	ExportFormatRFC5424 SIEMExportFormat = "RFC5424"
	ExportFormatJSON    SIEMExportFormat = "JSON"
)

// SIEMExporterConfig configures streaming to remote SIEM sinks.
type SIEMExporterConfig struct {
	EndpointURL   string           `json:"endpoint_url"`
	AuthHeader    string           `json:"auth_header"`
	Format        SIEMExportFormat `json:"format"`
	BatchSize     int              `json:"batch_size"`
	FlushInterval time.Duration    `json:"flush_interval"`
	Timeout       time.Duration    `json:"timeout"`
}

// DefaultSIEMExporterConfig returns production defaults for SIEM streaming.
func DefaultSIEMExporterConfig() SIEMExporterConfig {
	return SIEMExporterConfig{
		Format:        ExportFormatCEF,
		BatchSize:     50,
		FlushInterval: 1 * time.Second,
		Timeout:       5 * time.Second,
	}
}

// SIEMExporter handles batching and forwarding of audit events to remote SIEM endpoints.
type SIEMExporter struct {
	config     SIEMExporterConfig
	httpClient *http.Client
	streamer   *SIEMAuditStreamer
	buffer     []EnterpriseAuditEvent
	mu         sync.Mutex
	stopChan   chan struct{}
	wg         sync.WaitGroup
}

// NewSIEMExporter constructs an active SIEM exporter.
func NewSIEMExporter(streamer *SIEMAuditStreamer, cfg ...SIEMExporterConfig) *SIEMExporter {
	c := DefaultSIEMExporterConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}

	if streamer == nil {
		streamer = NewSIEMAuditStreamer()
	}

	exp := &SIEMExporter{
		config: c,
		httpClient: &http.Client{
			Timeout: c.Timeout,
		},
		streamer: streamer,
		buffer:   make([]EnterpriseAuditEvent, 0, c.BatchSize),
		stopChan: make(chan struct{}),
	}

	if c.EndpointURL != "" {
		exp.wg.Add(1)
		go exp.flusherLoop()
	}

	return exp
}

// Send emits an event into the exporter's streaming buffer.
func (e *SIEMExporter) Send(event EnterpriseAuditEvent) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.buffer = append(e.buffer, event)
	if len(e.buffer) >= e.config.BatchSize && e.config.EndpointURL != "" {
		go e.flushCurrent()
	}
	return nil
}

func (e *SIEMExporter) flusherLoop() {
	defer e.wg.Done()
	ticker := time.NewTicker(e.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopChan:
			e.flushCurrent()
			return
		case <-ticker.C:
			e.flushCurrent()
		}
	}
}

func (e *SIEMExporter) flushCurrent() {
	e.mu.Lock()
	if len(e.buffer) == 0 {
		e.mu.Unlock()
		return
	}
	batch := e.buffer
	e.buffer = make([]EnterpriseAuditEvent, 0, e.config.BatchSize)
	e.mu.Unlock()

	_ = e.postBatch(context.Background(), batch)
}

func (e *SIEMExporter) postBatch(ctx context.Context, batch []EnterpriseAuditEvent) error {
	if e.config.EndpointURL == "" || len(batch) == 0 {
		return nil
	}

	var payload []byte
	var contentType string

	switch e.config.Format {
	case ExportFormatCEF:
		var sb bytes.Buffer
		for _, ev := range batch {
			sb.WriteString(FormatCEF(ev))
			sb.WriteString("\n")
		}
		payload = sb.Bytes()
		contentType = "text/plain"
	case ExportFormatRFC5424:
		var sb bytes.Buffer
		for _, ev := range batch {
			sb.WriteString(FormatRFC5424(ev))
			sb.WriteString("\n")
		}
		payload = sb.Bytes()
		contentType = "text/plain"
	case ExportFormatJSON:
		var err error
		payload, err = json.Marshal(batch)
		if err != nil {
			return err
		}
		contentType = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.config.EndpointURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	if e.config.AuthHeader != "" {
		req.Header.Set("Authorization", e.config.AuthHeader)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("remote SIEM returned HTTP %d", resp.StatusCode)
	}

	return nil
}

// Close gracefully flushes pending events and stops background workers.
func (e *SIEMExporter) Close() error {
	close(e.stopChan)
	e.wg.Wait()
	return nil
}
