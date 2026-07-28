package dreamer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

// Gardener is the deterministic hygiene pass: it repairs what is mechanically
// safe (frontmatter) and reports what needs intelligence or a human — missing
// descriptions, broken links, notes in no hub, inbox stragglers. Reports don't
// consume budget; only writes do.
type Gardener struct {
	Store       *store.Store
	MaxInboxAge time.Duration

	now func() time.Time
}

func NewGardener(st *store.Store) *Gardener {
	return &Gardener{Store: st, MaxInboxAge: 24 * time.Hour, now: time.Now}
}

func (g *Gardener) Name() string { return "gardener" }

func (g *Gardener) Run(ctx context.Context, budget int) ([]string, error) {
	notes, err := g.Store.ListNotes()
	if err != nil {
		return nil, err
	}
	root, hubs, err := g.Store.Hubs()
	if err != nil {
		return nil, err
	}
	allHubs := append([]*vault.Note{root}, hubs...)

	var actions []string
	repairs := 0
	for _, h := range allHubs {
		if ctx.Err() != nil {
			return actions, ctx.Err()
		}
		if h.Frontmatter.Type == vault.TypeHub {
			continue
		}
		if repairs >= budget {
			actions = append(actions, fmt.Sprintf("repair budget (%d) exhausted; remaining repairs deferred", budget))
			break
		}
		h.Frontmatter.Type = vault.TypeHub
		if err := g.Store.Save(store.ActorDreamer, h); err != nil {
			return actions, fmt.Errorf("repairing type on %s: %w", h.Slug, err)
		}
		repairs++
		actions = append(actions, fmt.Sprintf("repaired frontmatter: set type hub on [[%s]]", h.Slug))
	}

	all := append(append([]*vault.Note{}, notes...), allHubs...)
	for _, n := range all {
		if n.Frontmatter.Description == "" {
			actions = append(actions, fmt.Sprintf("no description: [[%s]]", n.Slug))
		}
		for _, target := range n.Links() {
			if !g.Store.Exists(target) {
				actions = append(actions, fmt.Sprintf("broken link [[%s]] in [[%s]]", target, n.Slug))
			}
		}
	}

	inHub := make(map[string]bool)
	for _, h := range allHubs {
		for _, target := range h.Links() {
			inHub[target] = true
		}
	}
	for _, n := range notes {
		if !inHub[n.Slug] {
			actions = append(actions, fmt.Sprintf("in no hub: [[%s]]", n.Slug))
		}
	}

	inbox, err := g.Store.ListInbox()
	if err != nil {
		return actions, err
	}
	for _, n := range inbox {
		if age, ok := g.captureAge(n); ok && age > g.MaxInboxAge {
			actions = append(actions, fmt.Sprintf("inbox straggler: %s (unfiled for %dh)", n.Slug, int(age.Hours())))
		}
	}
	return actions, nil
}

// captureAge prefers the timestamp embedded in capture slugs
// (capture-YYYYMMDD-HHMMSS-xxxx) and falls back to the created date for
// foreign inbox files.
func (g *Gardener) captureAge(n *vault.Note) (time.Duration, bool) {
	const slugStamp = "20060102-150405"
	slug := n.Slug
	if strings.HasPrefix(slug, "capture-") && len(slug) > len("capture-")+len(slugStamp) {
		if t, err := time.ParseInLocation(slugStamp, slug[len("capture-"):len("capture-")+len(slugStamp)], time.Local); err == nil {
			return g.now().Sub(t), true
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", n.Frontmatter.Created, time.Local); err == nil {
		return g.now().Sub(t), true
	}
	return 0, false
}
