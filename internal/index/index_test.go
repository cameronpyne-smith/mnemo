package index

import (
	"reflect"
	"testing"

	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

func note(slug, folder, description, body string, tags ...string) *vault.Note {
	return &vault.Note{
		Slug:   slug,
		Folder: folder,
		Frontmatter: vault.Frontmatter{
			Description: description,
			Tags:        tags,
		},
		Body: body,
	}
}

func testIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	notes := []*vault.Note{
		note("birth-plan", vault.FolderNotes, "Plan for the hospital birth.", "Bag is in [[hospital-bag]]. Midwife notes.", "health"),
		note("hospital-bag", vault.FolderNotes, "Packing list for the hospital bag.", "Snacks, chargers. See [[birth-plan]].", "health"),
		note("sourdough-starter", vault.FolderNotes, "Feeding schedule for the sourdough starter.", "Feed daily at 8am.", "cooking"),
		note("health", vault.FolderHubs, "Hub for health notes.", "- [[birth-plan]] — the plan.\n- [[hospital-bag]] — the bag.\n"),
	}
	for _, n := range notes {
		if err := idx.IndexNote(n); err != nil {
			t.Fatalf("IndexNote(%s): %v", n.Slug, err)
		}
	}
	return idx
}

func TestSearchFindsByDescriptionAndBody(t *testing.T) {
	idx := testIndex(t)
	tests := []struct {
		query    string
		wantTop  string
		minHits  int
	}{
		{"hospital birth", "birth-plan", 1},
		{"packing list", "hospital-bag", 1},
		{"sourdough", "sourdough-starter", 1},
		{"midwife", "birth-plan", 1},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			hits, err := idx.Search(tt.query, 10)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(hits) < tt.minHits {
				t.Fatalf("got %d hits, want at least %d", len(hits), tt.minHits)
			}
			if hits[0].Slug != tt.wantTop {
				t.Errorf("top hit = %s, want %s", hits[0].Slug, tt.wantTop)
			}
			if hits[0].Description == "" || hits[0].Folder == "" {
				t.Errorf("hit missing stored fields: %+v", hits[0])
			}
		})
	}
}

func TestGraphEdges(t *testing.T) {
	idx := testIndex(t)
	if got := idx.Backlinks("birth-plan"); !reflect.DeepEqual(got, []string{"health", "hospital-bag"}) {
		t.Errorf("Backlinks(birth-plan) = %v", got)
	}
	if got := idx.Outbound("birth-plan"); !reflect.DeepEqual(got, []string{"hospital-bag"}) {
		t.Errorf("Outbound(birth-plan) = %v", got)
	}
}

func TestReindexReplacesEdges(t *testing.T) {
	idx := testIndex(t)
	updated := note("birth-plan", vault.FolderNotes, "Plan for the hospital birth.", "No more links.")
	if err := idx.IndexNote(updated); err != nil {
		t.Fatalf("IndexNote: %v", err)
	}
	if got := idx.Outbound("birth-plan"); len(got) != 0 {
		t.Errorf("Outbound after reindex = %v, want empty", got)
	}
	if got := idx.Backlinks("hospital-bag"); !reflect.DeepEqual(got, []string{"health"}) {
		t.Errorf("Backlinks(hospital-bag) = %v, want [health] (hub link remains)", got)
	}
}

func TestRemoveNote(t *testing.T) {
	idx := testIndex(t)
	if err := idx.RemoveNote("hospital-bag"); err != nil {
		t.Fatalf("RemoveNote: %v", err)
	}
	hits, err := idx.Search("packing", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, h := range hits {
		if h.Slug == "hospital-bag" {
			t.Error("removed note still in search results")
		}
	}
	for _, src := range idx.Backlinks("birth-plan") {
		if src == "hospital-bag" {
			t.Error("removed note still in backlinks")
		}
	}
	if idx.Count() != 3 {
		t.Errorf("Count = %d, want 3", idx.Count())
	}
}
