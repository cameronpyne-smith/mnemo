package dreamer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

// Pass is one background enrichment routine. Run performs at most budget
// actions (writes, LLM judgements) and returns one human-readable line per
// action taken or finding worth journaling. Passes write through the store
// only, as store.ActorDreamer.
type Pass interface {
	Name() string
	Run(ctx context.Context, budget int) ([]string, error)
}

type PassReport struct {
	Pass    string
	Actions []string
	Err     string
}

type Stats struct {
	Scheduled   bool
	Running     bool
	LastCycle   time.Time
	LastActions int
}

type Dreamer struct {
	Store     *store.Store
	Passes    []Pass
	Log       *slog.Logger
	IdleAfter time.Duration
	Interval  time.Duration
	Budget    int
	Scheduled bool

	now func() time.Time

	cycleMu sync.Mutex

	mu          sync.Mutex
	running     bool
	lastCycle   time.Time
	lastActions int
}

func New(st *store.Store, passes []Pass, log *slog.Logger) *Dreamer {
	return &Dreamer{
		Store:     st,
		Passes:    passes,
		Log:       log,
		IdleAfter: 30 * time.Minute,
		Interval:  6 * time.Hour,
		Budget:    20,
		now:       time.Now,
	}
}

func (d *Dreamer) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !d.due() {
			continue
		}
		if _, err := d.Dream(ctx); err != nil && !errors.Is(err, store.ErrUnavailable) {
			d.Log.Error("dream cycle failed", "error", err)
		}
	}
}

// due gates scheduled cycles: the vault must have been quiet for IdleAfter,
// the inbox drained (filing settled), and Interval elapsed since the last
// cycle. Manual Dream calls skip all of this.
func (d *Dreamer) due() bool {
	if d.now().Sub(d.Store.LastMutation()) < d.IdleAfter {
		return false
	}
	d.mu.Lock()
	last := d.lastCycle
	d.mu.Unlock()
	if !last.IsZero() && d.now().Sub(last) < d.Interval {
		return false
	}
	inbox, err := d.Store.ListInbox()
	if err != nil {
		d.Log.Error("dreamer inbox check failed", "error", err)
		return false
	}
	return len(inbox) == 0
}

// Dream runs every pass once, journals what they did, and returns the
// per-pass reports. Only one cycle runs at a time; a concurrent call fails
// with store.ErrUnavailable rather than queueing.
func (d *Dreamer) Dream(ctx context.Context) ([]PassReport, error) {
	if !d.cycleMu.TryLock() {
		return nil, fmt.Errorf("dream: a cycle is already running: %w", store.ErrUnavailable)
	}
	defer d.cycleMu.Unlock()

	d.mu.Lock()
	d.running = true
	d.mu.Unlock()

	start := d.now()
	total := 0
	reports := make([]PassReport, 0, len(d.Passes))
	for _, p := range d.Passes {
		if ctx.Err() != nil {
			break
		}
		actions, err := p.Run(ctx, d.Budget)
		r := PassReport{Pass: p.Name(), Actions: actions}
		if err != nil {
			r.Err = err.Error()
			d.Log.Error("dreamer pass failed", "pass", p.Name(), "error", err)
		}
		total += len(actions)
		reports = append(reports, r)
		d.Log.Info("dreamer pass done", "pass", p.Name(), "actions", len(actions))
	}
	if err := d.journal(reports); err != nil {
		d.Log.Error("writing dreamer journal", "error", err)
	}

	d.mu.Lock()
	d.running = false
	d.lastCycle = start
	d.lastActions = total
	d.mu.Unlock()
	return reports, nil
}

// journal appends this cycle's actions to the month's journal note in
// archive/ — in the vault so the record survives with the notes, in archive/
// so audit trail stays out of search and embeddings, like processed captures.
func (d *Dreamer) journal(reports []PassReport) error {
	var b strings.Builder
	stamp := d.now().Format("2006-01-02 15:04")
	for _, r := range reports {
		if len(r.Actions) == 0 && r.Err == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s — %s\n", stamp, r.Pass)
		for _, a := range r.Actions {
			fmt.Fprintf(&b, "- %s\n", a)
		}
		if r.Err != "" {
			fmt.Fprintf(&b, "- pass ended with error: %s\n", r.Err)
		}
	}
	if b.Len() == 0 {
		return nil
	}

	slug := "dreamer-journal-" + d.now().Format("2006-01")
	var n *vault.Note
	view, err := d.Store.Get(slug)
	switch {
	case err == nil:
		n = view.Note
	case errors.Is(err, store.ErrNotFound):
		n = &vault.Note{
			Slug:   slug,
			Folder: vault.FolderArchive,
			Frontmatter: vault.Frontmatter{
				Description: fmt.Sprintf("Dreamer action journal for %s.", d.now().Format("January 2006")),
			},
			Body: fmt.Sprintf("# Dreamer journal %s\n\n", d.now().Format("2006-01")),
		}
	default:
		return err
	}

	if n.Body != "" && !strings.HasSuffix(n.Body, "\n") {
		n.Body += "\n"
	}
	if n.Body != "" && !strings.HasSuffix(n.Body, "\n\n") {
		n.Body += "\n"
	}
	n.Body += b.String()
	return d.Store.Save(store.ActorDreamer, n)
}

func (d *Dreamer) Stats() Stats {
	d.mu.Lock()
	defer d.mu.Unlock()
	return Stats{
		Scheduled:   d.Scheduled,
		Running:     d.running,
		LastCycle:   d.lastCycle,
		LastActions: d.lastActions,
	}
}
