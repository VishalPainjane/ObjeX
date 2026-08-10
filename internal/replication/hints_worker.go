package replication

import (
	"context"
	"log/slog"
	"time"

	"github.com/VishalPainjane/objex/internal/metrics"
)

const (
	hintWorkerInterval = 2 * time.Second
	hintInitialBackoff = 1 * time.Second
	hintMaxBackoff     = 5 * time.Minute
	hintBatchSize      = 50
)

// HintWorker retries durable replication hints.
type HintWorker struct {
	coord  *Coordinator
	logger *slog.Logger
}

// NewHintWorker creates a background hint delivery worker.
func NewHintWorker(coord *Coordinator, logger *slog.Logger) *HintWorker {
	return &HintWorker{coord: coord, logger: logger}
}

// Run processes due hints until ctx is cancelled.
func (w *HintWorker) Run(ctx context.Context) {
	if w == nil || w.coord == nil || w.coord.MetaStore() == nil {
		return
	}
	ticker := time.NewTicker(hintWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processDue(ctx)
		}
	}
}

func (w *HintWorker) processDue(ctx context.Context) {
	meta := w.coord.MetaStore()
	now := time.Now().UTC()
	hints, err := meta.ListDueHints(ctx, now, hintBatchSize)
	if err != nil {
		if w.logger != nil {
			w.logger.Warn("list hints failed", "error", err)
		}
		return
	}
	if pending, err := meta.CountPendingHints(ctx); err == nil {
		metrics.SetHintsPending(pending)
	}

	for _, hint := range hints {
		start := time.Now()
		err := w.coord.DeliverHint(ctx, hint)
		if err == nil {
			_ = meta.MarkHintDelivered(ctx, hint.ID)
			w.coord.CleanupHintPayload(hint.SourcePath)
			metrics.RecordHintDelivered(string(hint.Kind))
			metrics.ObserveHintDelivery(time.Since(start))
			if w.logger != nil {
				w.logger.Info("hint delivered",
					"hint_target", hint.TargetNode,
					"version", hint.Version,
					"attempt", hint.Attempts,
					"result", "success",
					"size", 0,
				)
			}
			continue
		}
		next := nextHintAttempt(hint.Attempts)
		_ = meta.RecordHintFailure(ctx, hint.ID, next, err.Error())
		metrics.RecordHintFailed(string(hint.Kind))
		if w.logger != nil {
			w.logger.Warn("hint delivery failed",
				"hint_target", hint.TargetNode,
				"version", hint.Version,
				"attempt", hint.Attempts+1,
				"result", "failure",
				"error", err.Error(),
			)
		}
	}
}

func nextHintAttempt(attempts int) time.Time {
	delay := hintInitialBackoff
	for i := 0; i < attempts; i++ {
		delay *= 2
		if delay > hintMaxBackoff {
			delay = hintMaxBackoff
			break
		}
	}
	return time.Now().UTC().Add(delay)
}

// ProcessDueHintsOnce runs one hint batch (for tests).
func (w *HintWorker) ProcessDueHintsOnce(ctx context.Context) {
	w.processDue(ctx)
}
