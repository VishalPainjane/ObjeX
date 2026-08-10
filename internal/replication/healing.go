package replication

import (
	"context"
	"log/slog"
	"time"

	"github.com/VishalPainjane/objex/internal/metrics"
)

const (
	defaultHealingInterval = 60 * time.Second
	defaultHealingBatch    = 50
)

// HealingWorker scans primary objects marked partial and schedules repair hints.
type HealingWorker struct {
	coord    *Coordinator
	interval time.Duration
	batch    int
	logger   *slog.Logger
}

// NewHealingWorker creates a background healing scheduler.
func NewHealingWorker(coord *Coordinator, logger *slog.Logger) *HealingWorker {
	return &HealingWorker{
		coord:    coord,
		interval: defaultHealingInterval,
		batch:    defaultHealingBatch,
		logger:   logger,
	}
}

// Run scans until ctx is cancelled.
func (w *HealingWorker) Run(ctx context.Context) {
	if w == nil || w.coord == nil || !w.coord.Enabled() {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.ScanOnce(ctx)
		}
	}
}

// ScanOnce runs one healing batch (for tests and recovery hooks).
func (w *HealingWorker) ScanOnce(ctx context.Context) int {
	if w == nil || w.coord == nil {
		return 0
	}
	n, err := w.coord.HealPartialObjects(ctx, w.batch)
	if err != nil && w.logger != nil {
		w.logger.Warn("healing scan failed", "error", err)
	}
	if n > 0 {
		metrics.RecordHealingObjects(n)
	}
	return n
}
