package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	CrossProcessChannel         = "scandrix_cross_process_events"
	CrossProcessTable           = "scandrix_cross_process_events"
	BridgedFlag                 = "__scandrixBridged"
	ForwardFlushInterval        = 50 * time.Millisecond
	ForwardFlushMaxBatch        = 200
	DeliverInflightLimit        = 50
	DeliveredIDsLRUCap          = 10000
	PollInterval                = 5 * time.Second
	SweepInterval               = 15 * time.Minute
	RowTTLMinutes               = 60
)

// BridgeEnvelope carries an in-process or cross-process event payload.
type BridgeEnvelope struct {
	ID         int64          `json:"id,omitempty"`
	InstanceID string         `json:"instanceId"`
	Name       string         `json:"name"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  time.Time      `json:"createdAt,omitempty"`
}

// EventHandlerFunc handles a delivered cross-process event.
type EventHandlerFunc func(ctx context.Context, name string, payload map[string]any) error

// CrossProcessEventsBridge provides cross-process event delivery using PostgreSQL LISTEN/NOTIFY.
type CrossProcessEventsBridge struct {
	pool        *pgxpool.Pool
	dbURL       string
	instanceID  string
	subscribers map[string][]EventHandlerFunc
	subMu       sync.RWMutex

	forwardQueue chan *BridgeEnvelope
	deliveredLRU map[int64]time.Time
	lruMu        sync.Mutex

	inflightDeliveries int32
	pollLastSeenID     int64
	pollInitialized    bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	forwardedCount uint64
	deliveredCount uint64
	droppedCount   uint64
	notifyCount    uint64
}

// NewCrossProcessEventsBridge instantiates the bridge.
func NewCrossProcessEventsBridge(pool *pgxpool.Pool, dbURL string) *CrossProcessEventsBridge {
	ctx, cancel := context.WithCancel(context.Background())
	return &CrossProcessEventsBridge{
		pool:         pool,
		dbURL:        dbURL,
		instanceID:   uuid.New().String(),
		subscribers:  make(map[string][]EventHandlerFunc),
		forwardQueue: make(chan *BridgeEnvelope, 1000),
		deliveredLRU: make(map[int64]time.Time),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// EnsureSchema creates the events table and index if they do not exist.
func (b *CrossProcessEventsBridge) EnsureSchema(ctx context.Context) error {
	if b.pool == nil {
		return nil
	}

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL PRIMARY KEY,
			instance_id TEXT NOT NULL,
			name TEXT NOT NULL,
			payload JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_%s_id_instance ON %s (id, instance_id);
		CREATE INDEX IF NOT EXISTS idx_%s_created_at ON %s (created_at);
	`, CrossProcessTable, CrossProcessTable, CrossProcessTable, CrossProcessTable, CrossProcessTable)

	_, err := b.pool.Exec(ctx, query)
	return err
}

// Start launches the forward flush loop, LISTEN worker, fallback poller, and TTL sweeper.
func (b *CrossProcessEventsBridge) Start() error {
	if err := b.EnsureSchema(b.ctx); err != nil {
		return fmt.Errorf("failed ensuring cross-process events schema: %w", err)
	}

	// 1. Initialize polling water mark
	if b.pool != nil {
		var maxID *int64
		err := b.pool.QueryRow(b.ctx, fmt.Sprintf("SELECT MAX(id) FROM %s", CrossProcessTable)).Scan(&maxID)
		if err == nil && maxID != nil {
			b.pollLastSeenID = *maxID
		}
		b.pollInitialized = true
	}

	// 2. Start Forwarding Batch Flusher
	b.wg.Add(1)
	go b.forwardFlushLoop()

	// 3. Start LISTEN Listener Worker
	if b.dbURL != "" {
		b.wg.Add(1)
		go b.listenLoop()
	}

	// 4. Start Fallback Polling Loop
	if b.pool != nil {
		b.wg.Add(1)
		go b.pollLoop()
	}

	// 5. Start TTL Sweep Loop
	if b.pool != nil {
		b.wg.Add(1)
		go b.sweepLoop()
	}

	return nil
}

// Stop cleanly terminates all workers and flushes pending envelopes.
func (b *CrossProcessEventsBridge) Stop() {
	b.cancel()
	b.wg.Wait()
}

// Subscribe registers an event listener for a cross-process event name.
func (b *CrossProcessEventsBridge) Subscribe(name string, handler EventHandlerFunc) {
	b.subMu.Lock()
	defer b.subMu.Unlock()
	b.subscribers[name] = append(b.subscribers[name], handler)
}

// Forward buffers a local event for batched cross-process dispatch.
func (b *CrossProcessEventsBridge) Forward(ctx context.Context, name string, payload map[string]any) error {
	// Guard against infinite ping-pong if event was already bridged
	if _, ok := payload[BridgedFlag]; ok {
		return nil
	}

	envelope := &BridgeEnvelope{
		InstanceID: b.instanceID,
		Name:       name,
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}

	select {
	case b.forwardQueue <- envelope:
		atomic.AddUint64(&b.forwardedCount, 1)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Queue full: dispatch synchronously to avoid event loss
		return b.flushBatch([]*BridgeEnvelope{envelope})
	}
}

func (b *CrossProcessEventsBridge) forwardFlushLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(ForwardFlushInterval)
	defer ticker.Stop()

	var batch []*BridgeEnvelope

	for {
		select {
		case <-b.ctx.Done():
			if len(batch) > 0 {
				_ = b.flushBatch(batch)
			}
			return
		case env := <-b.forwardQueue:
			batch = append(batch, env)
			if len(batch) >= ForwardFlushMaxBatch {
				_ = b.flushBatch(batch)
				batch = nil
			}
		case <-ticker.C:
			if len(batch) > 0 {
				_ = b.flushBatch(batch)
				batch = nil
			}
		}
	}
}

func (b *CrossProcessEventsBridge) flushBatch(batch []*BridgeEnvelope) error {
	if len(batch) == 0 || b.pool == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
	defer cancel()

	valueStrings := make([]string, 0, len(batch))
	valueArgs := make([]any, 0, len(batch)*3)
	argIdx := 1

	for _, env := range batch {
		payloadBytes, err := json.Marshal(env.Payload)
		if err != nil {
			continue
		}
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d)", argIdx, argIdx+1, argIdx+2))
		valueArgs = append(valueArgs, env.InstanceID, env.Name, payloadBytes)
		argIdx += 3
	}

	if len(valueStrings) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (instance_id, name, payload)
		VALUES %s
		RETURNING id
	`, CrossProcessTable, strings.Join(valueStrings, ", "))

	rows, err := b.pool.Query(ctx, query, valueArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var insertedIDs []string
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			insertedIDs = append(insertedIDs, strconv.FormatInt(id, 10))
		}
	}

	if len(insertedIDs) > 0 {
		notifyPayload := strings.Join(insertedIDs, ",")
		notifyQuery := fmt.Sprintf("SELECT pg_notify('%s', $1)", CrossProcessChannel)
		_, _ = b.pool.Exec(ctx, notifyQuery, notifyPayload)
		atomic.AddUint64(&b.notifyCount, 1)
	}

	return nil
}

func (b *CrossProcessEventsBridge) listenLoop() {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			return
		default:
		}

		conn, err := pgx.Connect(b.ctx, b.dbURL)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		_, err = conn.Exec(b.ctx, fmt.Sprintf("LISTEN %s", CrossProcessChannel))
		if err != nil {
			_ = conn.Close(b.ctx)
			time.Sleep(2 * time.Second)
			continue
		}

		for {
			notification, err := conn.WaitForNotification(b.ctx)
			if err != nil {
				_ = conn.Close(b.ctx)
				break
			}

			b.handleNotification(notification)
		}
	}
}

func (b *CrossProcessEventsBridge) handleNotification(n *pgconn.Notification) {
	if n == nil || n.Payload == "" {
		return
	}

	// Cap concurrent in-flight deliveries
	inflight := atomic.AddInt32(&b.inflightDeliveries, 1)
	defer atomic.AddInt32(&b.inflightDeliveries, -1)

	if inflight > DeliverInflightLimit {
		atomic.AddUint64(&b.droppedCount, 1)
		return
	}

	rawIDs := strings.Split(n.Payload, ",")
	ids := make([]int64, 0, len(rawIDs))

	b.lruMu.Lock()
	for _, raw := range rawIDs {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err == nil {
			if _, exists := b.deliveredLRU[id]; !exists {
				ids = append(ids, id)
				b.deliveredLRU[id] = time.Now()
			}
		}
	}
	// Prune LRU if above capacity
	if len(b.deliveredLRU) > DeliveredIDsLRUCap {
		b.pruneLRU()
	}
	b.lruMu.Unlock()

	if len(ids) == 0 || b.pool == nil {
		return
	}

	b.deliverIDs(ids)
}

func (b *CrossProcessEventsBridge) deliverIDs(ids []int64) {
	ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT id, instance_id, name, payload
		FROM %s
		WHERE id = ANY($1)
	`, CrossProcessTable)

	rows, err := b.pool.Query(ctx, query, ids)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var instID, name string
		var payloadBytes []byte

		if err := rows.Scan(&id, &instID, &name, &payloadBytes); err != nil {
			continue
		}

		// Self-delivery guard: skip envelopes emitted by this same instance
		if instID == b.instanceID {
			continue
		}

		var payload map[string]any
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			continue
		}

		payload[BridgedFlag] = true
		b.dispatchLocal(name, payload)
		atomic.AddUint64(&b.deliveredCount, 1)
	}
}

func (b *CrossProcessEventsBridge) pollLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.pollMissedEvents()
		}
	}
}

func (b *CrossProcessEventsBridge) pollMissedEvents() {
	if b.pool == nil || !b.pollInitialized {
		return
	}

	ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT id, instance_id, name, payload
		FROM %s
		WHERE id > $1 AND instance_id != $2
		ORDER BY id ASC
		LIMIT 100
	`, CrossProcessTable)

	rows, err := b.pool.Query(ctx, query, b.pollLastSeenID, b.instanceID)
	if err != nil {
		return
	}
	defer rows.Close()

	var maxSeen int64 = b.pollLastSeenID

	for rows.Next() {
		var id int64
		var instID, name string
		var payloadBytes []byte

		if err := rows.Scan(&id, &instID, &name, &payloadBytes); err != nil {
			continue
		}

		if id > maxSeen {
			maxSeen = id
		}

		// Check LRU
		b.lruMu.Lock()
		if _, exists := b.deliveredLRU[id]; exists {
			b.lruMu.Unlock()
			continue
		}
		b.deliveredLRU[id] = time.Now()
		b.lruMu.Unlock()

		var payload map[string]any
		if err := json.Unmarshal(payloadBytes, &payload); err == nil {
			payload[BridgedFlag] = true
			b.dispatchLocal(name, payload)
			atomic.AddUint64(&b.deliveredCount, 1)
		}
	}

	b.pollLastSeenID = maxSeen
}

func (b *CrossProcessEventsBridge) sweepLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(SweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
			threshold := time.Now().UTC().Add(-time.Duration(RowTTLMinutes) * time.Minute)
			query := fmt.Sprintf("DELETE FROM %s WHERE created_at < $1", CrossProcessTable)
			_, _ = b.pool.Exec(ctx, query, threshold)
			cancel()
		}
	}
}

func (b *CrossProcessEventsBridge) dispatchLocal(name string, payload map[string]any) {
	b.subMu.RLock()
	handlers := b.subscribers[name]
	b.subMu.RUnlock()

	for _, h := range handlers {
		go func(fn EventHandlerFunc) {
			_ = fn(b.ctx, name, payload)
		}(h)
	}
}

func (b *CrossProcessEventsBridge) pruneLRU() {
	now := time.Now()
	for k, v := range b.deliveredLRU {
		if now.Sub(v) > 30*time.Minute {
			delete(b.deliveredLRU, k)
		}
	}
}

// GetMetrics returns snapshot of bridge activity.
func (b *CrossProcessEventsBridge) GetMetrics() map[string]uint64 {
	return map[string]uint64{
		"forwarded": atomic.LoadUint64(&b.forwardedCount),
		"delivered": atomic.LoadUint64(&b.deliveredCount),
		"dropped":   atomic.LoadUint64(&b.droppedCount),
		"notifies":  atomic.LoadUint64(&b.notifyCount),
	}
}
