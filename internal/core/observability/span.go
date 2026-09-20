package observability

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

type simpleSpan struct {
	mu           sync.RWMutex
	name         string
	context      SpanContext
	parentSpanID string
	kind         SpanKind
	startTime    time.Time
	endTime      time.Time
	ended        bool
	status       SpanStatus
	attributes   map[string]any
	events       []SpanEvent
	links        []SpanLink
	resource     map[string]string
	onEnd        func(span *SpanData)
}

func newSpan(
	sc SpanContext,
	parentSpanID string,
	name string,
	opts SpanOptions,
	resource map[string]string,
	onEnd func(span *SpanData),
) *simpleSpan {
	kind := opts.Kind
	if kind == "" {
		kind = SpanKindInternal
	}
	startTime := opts.StartTime
	if startTime.IsZero() {
		startTime = time.Now().UTC()
	}

	attrs := make(map[string]any, len(opts.Attributes))
	for k, v := range opts.Attributes {
		attrs[k] = v
	}

	links := make([]SpanLink, len(opts.Links))
	copy(links, opts.Links)

	res := make(map[string]string, len(resource))
	for k, v := range resource {
		res[k] = v
	}

	return &simpleSpan{
		name:         name,
		context:      sc,
		parentSpanID: parentSpanID,
		kind:         kind,
		startTime:    startTime,
		status:       SpanStatus{Code: StatusOK},
		attributes:   attrs,
		events:       make([]SpanEvent, 0, 4),
		links:        links,
		resource:     res,
		onEnd:        onEnd,
	}
}

func (s *simpleSpan) SpanContext() SpanContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.context
}

func (s *simpleSpan) IsRecording() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.ended
}

func (s *simpleSpan) SetAttribute(key string, value any) Span {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return s
	}
	s.attributes[key] = value
	return s
}

func (s *simpleSpan) SetAttributes(attributes map[string]any) Span {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return s
	}
	for k, v := range attributes {
		s.attributes[k] = v
	}
	return s
}

func (s *simpleSpan) SetStatus(code StatusCode, description string) Span {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return s
	}
	s.status = SpanStatus{
		Code:        code,
		Description: description,
	}
	return s
}

func (s *simpleSpan) RecordError(err error) Span {
	if err == nil {
		return s
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return s
	}

	buf := make([]byte, 2048)
	n := runtime.Stack(buf, false)
	stack := string(buf[:n])

	event := SpanEvent{
		Name:      "exception",
		Timestamp: time.Now().UTC(),
		Attributes: map[string]any{
			"exception.type":    fmt.Sprintf("%T", err),
			"exception.message": err.Error(),
			"exception.stack":   stack,
		},
	}
	s.events = append(s.events, event)
	s.status = SpanStatus{
		Code:        StatusError,
		Description: err.Error(),
	}
	return s
}

func (s *simpleSpan) AddEvent(name string, attributes map[string]any) Span {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return s
	}
	attrs := make(map[string]any, len(attributes))
	for k, v := range attributes {
		attrs[k] = v
	}
	s.events = append(s.events, SpanEvent{
		Name:       name,
		Timestamp:  time.Now().UTC(),
		Attributes: attrs,
	})
	return s
}

func (s *simpleSpan) End() {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	s.endTime = time.Now().UTC()
	data := s.snapshotLocked()
	onEnd := s.onEnd
	s.mu.Unlock()

	if onEnd != nil {
		onEnd(data)
	}
}

func (s *simpleSpan) Snapshot() *SpanData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *simpleSpan) snapshotLocked() *SpanData {
	endTime := s.endTime
	if endTime.IsZero() {
		endTime = time.Now().UTC()
	}
	durationMs := endTime.Sub(s.startTime).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}

	attrs := make(map[string]any, len(s.attributes))
	for k, v := range s.attributes {
		attrs[k] = v
	}

	events := make([]SpanEvent, len(s.events))
	copy(events, s.events)

	links := make([]SpanLink, len(s.links))
	copy(links, s.links)

	res := make(map[string]string, len(s.resource))
	for k, v := range s.resource {
		res[k] = v
	}

	return &SpanData{
		Name:         s.name,
		Context:      s.context,
		ParentSpanID: s.parentSpanID,
		Kind:         s.kind,
		StartTime:    s.startTime,
		EndTime:      endTime,
		DurationMs:   durationMs,
		Attributes:   attrs,
		Events:       events,
		Links:        links,
		Status:       s.status,
		Resource:     res,
	}
}
