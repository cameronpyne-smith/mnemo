package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Worker struct {
	Filer    *Filer
	Interval time.Duration
	Log      *slog.Logger

	mu       sync.Mutex
	inflight map[string]bool
	wake     chan struct{}

	Processed int
	Failed    int
}

func NewWorker(filer *Filer, log *slog.Logger) *Worker {
	return &Worker{
		Filer:    filer,
		Interval: 15 * time.Second,
		Log:      log,
		inflight: make(map[string]bool),
		wake:     make(chan struct{}, 1),
	}
}

func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		w.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.wake:
		}
	}
}

func (w *Worker) drain(ctx context.Context) {
	notes, err := w.Filer.Store.ListInbox()
	if err != nil {
		w.Log.Error("scanning inbox", "error", err)
		return
	}
	for _, n := range notes {
		if ctx.Err() != nil {
			return
		}
		w.mu.Lock()
		if w.inflight[n.Slug] {
			w.mu.Unlock()
			continue
		}
		w.inflight[n.Slug] = true
		w.mu.Unlock()

		start := time.Now()
		result, err := w.Filer.File(ctx, n.Slug)

		w.mu.Lock()
		delete(w.inflight, n.Slug)
		if err != nil {
			w.Failed++
		} else {
			w.Processed++
		}
		w.mu.Unlock()

		if err != nil {
			w.Log.Error("filing failed; capture left in inbox", "capture", n.Slug, "error", err)
			continue
		}
		w.Log.Info("filed capture",
			"capture", n.Slug,
			"into", result.FiledInto,
			"summary", result.Summary,
			"turns", result.Turns,
			"took", time.Since(start).Round(time.Millisecond),
		)
	}
}

func (w *Worker) Stats() (processed, failed, inflight int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Processed, w.Failed, len(w.inflight)
}
