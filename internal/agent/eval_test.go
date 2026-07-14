package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cameronpyne-smith/mnemo/internal/ollama"
	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

type evalCase struct {
	name     string
	dumps    []string
	searches map[string][]string
	maxNotes int
}

var evalCases = []evalCase{
	{
		name:  "simple task",
		dumps: []string{"Buy more vanilla protein powder, the MyProtein one"},
		searches: map[string][]string{
			"protein powder": {"protein", "MyProtein"},
		},
	},
	{
		name:  "appointment",
		dumps: []string{"Dentist appointment moved from Tuesday to Friday 3pm, remember to bring the insurance letter"},
		searches: map[string][]string{
			"dentist appointment": {"Friday", "3pm", "insurance"},
		},
	},
	{
		name: "two related dumps should converge",
		dumps: []string{
			"Started reading Dune by Frank Herbert, about 50 pages in",
			"Finished Dune. Loved the ecology themes and the fremen culture, want to read the sequel",
		},
		searches: map[string][]string{
			"Dune Frank Herbert": {"ecology", "sequel"},
		},
		maxNotes: 2,
	},
}

func TestEvalFiling(t *testing.T) {
	base := os.Getenv("MNEMO_OLLAMA_TESTS")
	if base == "" {
		t.Skip("set MNEMO_OLLAMA_TESTS=http://localhost:11434 to run the filing eval")
	}
	model := os.Getenv("MNEMO_OLLAMA_MODEL")
	if model == "" {
		model = "qwen3:30b-a3b"
	}

	var passed, total int
	for _, ec := range evalCases {
		t.Run(ec.name, func(t *testing.T) {
			root := t.TempDir()
			if _, err := vault.Init(root); err != nil {
				t.Fatalf("vault.Init: %v", err)
			}
			st, err := store.Open(root)
			if err != nil {
				t.Fatalf("store.Open: %v", err)
			}
			f := &Filer{Store: st, LLM: ollama.New(base), Model: model}

			for _, dump := range ec.dumps {
				slug, err := st.Capture(dump, "eval")
				if err != nil {
					t.Fatalf("Capture: %v", err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				result, err := f.File(ctx, slug)
				cancel()
				if err != nil {
					t.Fatalf("File(%q): %v", dump, err)
				}
				t.Logf("filed %q into %v (%d turns): %s", dump, result.FiledInto, result.Turns, result.Summary)
			}

			check := func(name string, ok bool, detail string) {
				total++
				if ok {
					passed++
					t.Logf("PASS %s", name)
				} else {
					t.Errorf("FAIL %s: %s", name, detail)
				}
			}

			inbox, err := st.ListInbox()
			if err != nil {
				t.Fatalf("ListInbox: %v", err)
			}
			check("inbox drained", len(inbox) == 0, fmt.Sprintf("%d items left", len(inbox)))

			status, err := st.Status()
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			check("notes created", status.Notes >= 1, "no notes in notes/")
			if ec.maxNotes > 0 {
				check("no fragmentation", status.Notes <= ec.maxNotes,
					fmt.Sprintf("%d notes, want <= %d", status.Notes, ec.maxNotes))
			}

			for query, keywords := range ec.searches {
				hits, err := st.Search(query, 5)
				if err != nil {
					t.Fatalf("Search(%q): %v", query, err)
				}
				if len(hits) == 0 {
					check("search '"+query+"'", false, "no hits")
					continue
				}
				check("search '"+query+"'", true, "")

				var bodies strings.Builder
				for _, h := range hits {
					if view, err := st.Get(h.Slug); err == nil {
						bodies.WriteString(view.Note.Body)
						bodies.WriteString(view.Note.Frontmatter.Description)
					}
				}
				for _, kw := range keywords {
					check(fmt.Sprintf("fact %q preserved", kw),
						strings.Contains(strings.ToLower(bodies.String()), strings.ToLower(kw)),
						"keyword missing from filed notes")
				}
			}

			reachable := 0
			_, hubs, err := st.Hubs()
			if err != nil {
				t.Fatalf("Hubs: %v", err)
			}
			linked := make(map[string]bool)
			for _, h := range hubs {
				for _, l := range h.Links() {
					linked[l] = true
				}
			}
			notes, err := st.ListNotes()
			if err != nil {
				t.Fatalf("ListNotes: %v", err)
			}
			for _, n := range notes {
				if linked[n.Slug] {
					reachable++
				}
			}
			check("notes reachable from hubs", len(notes) > 0 && reachable == len(notes),
				fmt.Sprintf("%d/%d notes in hubs", reachable, len(notes)))
		})
	}
	if total > 0 {
		t.Logf("EVAL SCORE: %d/%d (%.0f%%) — model %s", passed, total, 100*float64(passed)/float64(total), model)
	}
}
