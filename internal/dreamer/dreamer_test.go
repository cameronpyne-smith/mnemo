package dreamer

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/cameronpyne-smith/mnemo/internal/index"
	"github.com/cameronpyne-smith/mnemo/internal/ollama"
	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

func testStore(t *testing.T) *store.Store {
	t.Helper()
	root := t.TempDir()
	if _, err := vault.Init(root); err != nil {
		t.Fatalf("vault.Init: %v", err)
	}
	st, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	return st
}

type fakePass struct {
	name      string
	actions   []string
	err       error
	gotBudget int
	runs      int
}

func (f *fakePass) Name() string { return f.name }

func (f *fakePass) Run(ctx context.Context, budget int) ([]string, error) {
	f.gotBudget = budget
	f.runs++
	return f.actions, f.err
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
}

func TestDreamRunsPassesAndJournals(t *testing.T) {
	st := testStore(t)
	busy := &fakePass{name: "one", actions: []string{"did a thing"}}
	idle := &fakePass{name: "two"}
	d := New(st, []Pass{busy, idle}, discardLog())
	d.Budget = 7
	d.now = fixedNow

	reports, err := d.Dream(context.Background())
	if err != nil {
		t.Fatalf("Dream: %v", err)
	}
	if len(reports) != 2 || reports[0].Pass != "one" || reports[1].Pass != "two" {
		t.Fatalf("reports = %+v", reports)
	}
	if busy.gotBudget != 7 {
		t.Errorf("budget passed = %d, want 7", busy.gotBudget)
	}

	view, err := st.Get("dreamer-journal-2026-07")
	if err != nil {
		t.Fatalf("journal note: %v", err)
	}
	if view.Note.Folder != vault.FolderArchive {
		t.Errorf("journal folder = %s, want archive", view.Note.Folder)
	}
	if !strings.Contains(view.Note.Body, "## 2026-07-26 12:00 — one") || !strings.Contains(view.Note.Body, "- did a thing") {
		t.Errorf("journal body missing entry:\n%s", view.Note.Body)
	}
	if strings.Contains(view.Note.Body, "two") {
		t.Errorf("empty pass should not be journaled:\n%s", view.Note.Body)
	}

	stats := d.Stats()
	if stats.LastActions != 1 || !stats.LastCycle.Equal(fixedNow()) || stats.Running {
		t.Errorf("stats = %+v", stats)
	}

	if _, err := d.Dream(context.Background()); err != nil {
		t.Fatalf("second Dream: %v", err)
	}
	view, err = st.Get("dreamer-journal-2026-07")
	if err != nil {
		t.Fatalf("journal note after second cycle: %v", err)
	}
	if got := strings.Count(view.Note.Body, "did a thing"); got != 2 {
		t.Errorf("journal entries = %d, want 2:\n%s", got, view.Note.Body)
	}
	if busy.runs != 2 {
		t.Errorf("pass runs = %d, want 2", busy.runs)
	}
}

func TestDreamQuietCycleWritesNoJournal(t *testing.T) {
	st := testStore(t)
	d := New(st, []Pass{&fakePass{name: "quiet"}}, discardLog())
	d.now = fixedNow

	if _, err := d.Dream(context.Background()); err != nil {
		t.Fatalf("Dream: %v", err)
	}
	if _, err := st.Get("dreamer-journal-2026-07"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("journal for quiet cycle: err = %v, want ErrNotFound", err)
	}
}

func TestDreamJournalsPassError(t *testing.T) {
	st := testStore(t)
	failing := &fakePass{name: "broken", err: errors.New("model exploded")}
	d := New(st, []Pass{failing}, discardLog())
	d.now = fixedNow

	reports, err := d.Dream(context.Background())
	if err != nil {
		t.Fatalf("Dream: %v", err)
	}
	if reports[0].Err != "model exploded" {
		t.Fatalf("reports = %+v", reports)
	}
	view, err := st.Get("dreamer-journal-2026-07")
	if err != nil {
		t.Fatalf("journal note: %v", err)
	}
	if !strings.Contains(view.Note.Body, "pass ended with error: model exploded") {
		t.Errorf("journal body missing error:\n%s", view.Note.Body)
	}
}

func TestDue(t *testing.T) {
	st := testStore(t)
	d := New(st, nil, discardLog())
	d.IdleAfter = 0
	d.Interval = 0
	if !d.due() {
		t.Error("idle store with empty inbox: due() = false, want true")
	}

	d.IdleAfter = time.Hour
	if d.due() {
		t.Error("store mutated within IdleAfter: due() = true, want false")
	}

	d.IdleAfter = 0
	if _, err := st.Capture("test", "pending dump", ""); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if d.due() {
		t.Error("non-empty inbox: due() = true, want false")
	}
}

func TestDueRespectsInterval(t *testing.T) {
	st := testStore(t)
	d := New(st, nil, discardLog())
	d.IdleAfter = 0
	d.Interval = time.Hour
	d.lastCycle = time.Now().Add(-time.Minute)
	if d.due() {
		t.Error("cycle ran within Interval: due() = true, want false")
	}
	d.lastCycle = time.Now().Add(-2 * time.Hour)
	if !d.due() {
		t.Error("Interval elapsed: due() = false, want true")
	}
}

func TestGardener(t *testing.T) {
	root := t.TempDir()
	v, err := vault.Init(root)
	if err != nil {
		t.Fatalf("vault.Init: %v", err)
	}
	fixtures := []*vault.Note{
		{Slug: "health", Folder: vault.FolderHubs,
			Frontmatter: vault.Frontmatter{Description: "Health hub."},
			Body:        "# health\n- [[a]] — a note\n"},
		{Slug: "a", Folder: vault.FolderNotes,
			Frontmatter: vault.Frontmatter{Description: "A."},
			Body:        "links to [[missing-note]]\n"},
		{Slug: "b", Folder: vault.FolderNotes,
			Body: "plain\n"},
		{Slug: "capture-20250101-000000-ab", Folder: vault.FolderInbox,
			Frontmatter: vault.Frontmatter{Description: "Old capture.", Created: "2025-01-01"},
			Body:        "stale\n"},
	}
	for _, n := range fixtures {
		if err := v.Write(n); err != nil {
			t.Fatalf("writing fixture %s: %v", n.Slug, err)
		}
	}
	st, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	g := NewGardener(st)
	g.now = fixedNow
	actions, err := g.Run(context.Background(), 10)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{
		"repaired frontmatter: set type hub on [[health]]",
		"no description: [[b]]",
		"broken link [[missing-note]] in [[a]]",
		"in no hub: [[b]]",
		"inbox straggler: capture-20250101-000000-ab",
	}
	joined := strings.Join(actions, "\n")
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("actions missing %q:\n%s", w, joined)
		}
	}
	if strings.Contains(joined, "in no hub: [[a]]") {
		t.Errorf("hub-listed note reported as orphan:\n%s", joined)
	}

	view, err := st.Get("health")
	if err != nil {
		t.Fatalf("Get health: %v", err)
	}
	if view.Note.Frontmatter.Type != vault.TypeHub {
		t.Errorf("hub type not repaired: %q", view.Note.Frontmatter.Type)
	}

	actions, err = g.Run(context.Background(), 10)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if strings.Contains(strings.Join(actions, "\n"), "repaired frontmatter") {
		t.Errorf("repair reported again on clean vault:\n%s", strings.Join(actions, "\n"))
	}
}

type fakeLLM struct {
	replies []string
	reqs    []ollama.ChatRequest
}

func (f *fakeLLM) Chat(ctx context.Context, req ollama.ChatRequest) (*ollama.ChatResponse, error) {
	f.reqs = append(f.reqs, req)
	reply := ""
	if n := len(f.reqs) - 1; n < len(f.replies) {
		reply = f.replies[n]
	}
	return &ollama.ChatResponse{Message: ollama.Message{Role: ollama.RoleAssistant, Content: reply}}, nil
}

func TestLinkerEmptyVault(t *testing.T) {
	st := testStore(t)
	actions, err := NewLinker(st, &fakeLLM{}, "test-model").Run(context.Background(), 10)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("actions = %v, want none", actions)
	}
}

func TestLinkerUnavailableWithoutEmbeddings(t *testing.T) {
	st := testStore(t)
	if err := st.Save("test", &vault.Note{
		Slug: "a", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "A."}, Body: "x\n",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := NewLinker(st, &fakeLLM{}, "test-model").Run(context.Background(), 10); !errors.Is(err, store.ErrUnavailable) {
		t.Fatalf("Run without embeddings: err = %v, want ErrUnavailable", err)
	}
}

func TestSelectCandidates(t *testing.T) {
	similar := map[string][]index.Hit{
		"a": {
			{Slug: "some-hub", Folder: vault.FolderHubs, Score: 0.95},
			{Slug: "b", Folder: vault.FolderNotes, Score: 0.9},
			{Slug: "c", Folder: vault.FolderNotes, Score: 0.5},
			{Slug: "e", Folder: vault.FolderNotes, Score: 0.2},
		},
		"b": {
			{Slug: "a", Folder: vault.FolderNotes, Score: 0.9},
			{Slug: "d", Folder: vault.FolderNotes, Score: 0.7},
		},
	}
	linked := map[[2]string]bool{pairKey("c", "a"): true}

	got := selectCandidates(similar, linked, 10)
	want := []candidate{
		{a: "a", b: "b", score: 0.9},
		{a: "b", b: "d", score: 0.7},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

func TestSelectCandidatesBudget(t *testing.T) {
	similar := map[string][]index.Hit{
		"a": {
			{Slug: "b", Folder: vault.FolderNotes, Score: 0.9},
			{Slug: "c", Folder: vault.FolderNotes, Score: 0.5},
		},
	}
	got := selectCandidates(similar, map[[2]string]bool{}, 1)
	if len(got) != 1 || got[0] != (candidate{a: "a", b: "b", score: 0.9}) {
		t.Errorf("candidates = %v, want the single best-scoring pair", got)
	}
}

func TestJudgeCandidates(t *testing.T) {
	st := testStore(t)
	notes := []*vault.Note{
		{Slug: "coffee", Folder: vault.FolderNotes,
			Frontmatter: vault.Frontmatter{Description: "Coffee brewing."}, Body: "V60 recipe.\n"},
		{Slug: "espresso", Folder: vault.FolderNotes,
			Frontmatter: vault.Frontmatter{Description: "Espresso dialing."}, Body: "18g in, 36g out.\n"},
		{Slug: "risotto", Folder: vault.FolderNotes,
			Frontmatter: vault.Frontmatter{Description: "Risotto method."}, Body: "Stir constantly.\n"},
	}
	for _, n := range notes {
		if err := st.Save("test", n); err != nil {
			t.Fatalf("Save %s: %v", n.Slug, err)
		}
	}

	llm := &fakeLLM{replies: []string{
		`{"link": true, "reason": "dialing espresso builds on brewing basics", "into": "espresso"}`,
		`{"link": false, "reason": "different domains", "into": "espresso"}`,
		`**No.** not json at all`,
	}}
	l := NewLinker(st, llm, "test-model")
	pairs := []candidate{
		{a: "coffee", b: "espresso", score: 0.9},
		{a: "espresso", b: "risotto", score: 0.7},
		{a: "coffee", b: "risotto", score: 0.5},
	}
	actions, err := l.judgeCandidates(context.Background(), pairs)
	if err != nil {
		t.Fatalf("judgeCandidates: %v", err)
	}
	want := []string{
		"linked [[espresso]] → [[coffee]] (0.90): dialing espresso builds on brewing basics",
		"skip [[espresso]] ↔ [[risotto]] (0.70): different domains",
		"skip [[coffee]] ↔ [[risotto]] (0.50): unparseable verdict: **No.** not json at all",
	}
	if len(actions) != len(want) {
		t.Fatalf("actions = %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Errorf("actions[%d] = %q, want %q", i, actions[i], want[i])
		}
	}

	view, err := st.Get("espresso")
	if err != nil {
		t.Fatalf("Get espresso: %v", err)
	}
	if !strings.Contains(view.Note.Body, "## Related") || !strings.Contains(view.Note.Body, "- [[coffee]] — dialing espresso builds on brewing basics") {
		t.Errorf("link not written into espresso:\n%s", view.Note.Body)
	}
	coffee, err := st.Get("coffee")
	if err != nil {
		t.Fatalf("Get coffee: %v", err)
	}
	if strings.Contains(coffee.Note.Body, "## Related") {
		t.Errorf("link written into wrong side:\n%s", coffee.Note.Body)
	}

	if len(llm.reqs) != 3 {
		t.Fatalf("chat calls = %d, want 3", len(llm.reqs))
	}
	req := llm.reqs[0]
	if req.Model != "test-model" || len(req.Tools) != 0 {
		t.Errorf("request model/tools = %q/%v", req.Model, req.Tools)
	}
	for _, part := range []string{`"link"`, `"enum"`, `"coffee"`, `"espresso"`} {
		if !strings.Contains(string(req.Format), part) {
			t.Errorf("request format missing %s: %s", part, req.Format)
		}
	}
	format := string(req.Format)
	reasonAt, linkAt, intoAt := strings.Index(format, `"reason"`), strings.Index(format, `"link"`), strings.Index(format, `"into"`)
	if !(reasonAt < linkAt && linkAt < intoAt) {
		t.Errorf("schema field order must be reason, link, into (generation order = reasoning before verdict): %s", format)
	}
	if len(req.Messages) != 2 || req.Messages[0].Role != ollama.RoleSystem || req.Messages[1].Role != ollama.RoleUser {
		t.Fatalf("messages = %+v", req.Messages)
	}
	user := req.Messages[1].Content
	for _, want := range []string{"V60 recipe.", "18g in, 36g out.", "[[coffee]]", "[[espresso]]"} {
		if !strings.Contains(user, want) {
			t.Errorf("user message missing %q:\n%s", want, user)
		}
	}

	again, err := l.judgeCandidates(context.Background(), pairs)
	if err != nil {
		t.Fatalf("second judgeCandidates: %v", err)
	}
	if len(llm.reqs) != 4 {
		t.Errorf("chat calls after re-run = %d, want 4 (only the unparseable pair re-judged)", len(llm.reqs))
	}
	if len(again) != 1 || !strings.HasPrefix(again[0], "skip [[coffee]] ↔ [[risotto]]") {
		t.Errorf("re-run actions = %v, want only the unparseable pair", again)
	}
}

func TestGardenerBudget(t *testing.T) {
	root := t.TempDir()
	v, err := vault.Init(root)
	if err != nil {
		t.Fatalf("vault.Init: %v", err)
	}
	broken := &vault.Note{Slug: "typeless", Folder: vault.FolderHubs,
		Frontmatter: vault.Frontmatter{Description: "Hub missing its type."},
		Body:        "# typeless\n"}
	if err := v.Write(broken); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	st, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	g := NewGardener(st)
	g.now = fixedNow
	actions, err := g.Run(context.Background(), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	joined := strings.Join(actions, "\n")
	if !strings.Contains(joined, "repair budget (0) exhausted") {
		t.Errorf("missing budget message:\n%s", joined)
	}
	view, err := st.Get("typeless")
	if err != nil {
		t.Fatalf("Get typeless: %v", err)
	}
	if view.Note.Frontmatter.Type == vault.TypeHub {
		t.Error("repair happened despite zero budget")
	}
}
