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

func TestVectorBookkeeping(t *testing.T) {
	idx := testIndex(t)
	idx.SetVectors("birth-plan", [][]float32{{1, 0}, {0, 1}})
	idx.SetVectors("hospital-bag", [][]float32{{0, 1}})

	got := idx.Vectors()
	if len(got) != 3 || got[0].Slug != "birth-plan" || got[1].Slug != "birth-plan" || got[2].Slug != "hospital-bag" {
		t.Errorf("Vectors = %v, want birth-plan's two chunks then hospital-bag", got)
	}
	if n := idx.VectorCount(); n != 2 {
		t.Errorf("VectorCount = %d, want 2 notes regardless of chunks", n)
	}

	idx.RemoveVectors("birth-plan")
	if got := idx.Vectors(); len(got) != 1 || got[0].Slug != "hospital-bag" {
		t.Errorf("Vectors after RemoveVectors = %v", got)
	}

	if err := idx.RemoveNote("hospital-bag"); err != nil {
		t.Fatalf("RemoveNote: %v", err)
	}
	if got := idx.Vectors(); len(got) != 0 {
		t.Errorf("Vectors after RemoveNote = %v, want empty", got)
	}
}

func TestVectorRankScoresNoteByBestChunk(t *testing.T) {
	idx := testIndex(t)
	idx.SetVectors("birth-plan", [][]float32{{0, 1}, {1, 0}})
	idx.SetVectors("hospital-bag", [][]float32{{0.9, 0.44}})

	got := idx.vectorRank([]float32{1, 0}, 10)
	if !reflect.DeepEqual(got, []string{"birth-plan", "hospital-bag"}) {
		t.Errorf("vectorRank = %v, want birth-plan (best chunk 1.0) then hospital-bag, no duplicates", got)
	}
}

func TestSearchHybridMergesBothSides(t *testing.T) {
	idx := testIndex(t)
	idx.SetVectors("birth-plan", [][]float32{{1, 0}})
	idx.SetVectors("hospital-bag", [][]float32{{0.6, 0.8}})
	idx.SetVectors("sourdough-starter", [][]float32{{0, 1}})

	hits, err := idx.SearchHybrid("packing", []float32{1, 0}, 10)
	if err != nil {
		t.Fatalf("SearchHybrid: %v", err)
	}
	var slugs []string
	for _, h := range hits {
		slugs = append(slugs, h.Slug)
	}
	if len(slugs) < 3 || slugs[0] != "hospital-bag" || slugs[1] != "birth-plan" {
		t.Fatalf("hybrid slugs = %v, want hospital-bag (both sides) then birth-plan (vector only)", slugs)
	}
	for _, h := range hits {
		if h.Folder == "" || h.Description == "" || h.Score <= 0 {
			t.Errorf("hit missing metadata: %+v", h)
		}
	}
}

func TestSimilar(t *testing.T) {
	idx := testIndex(t)
	idx.SetVectors("birth-plan", [][]float32{{1, 0}})
	idx.SetVectors("hospital-bag", [][]float32{{0.8, 0.6}})
	idx.SetVectors("sourdough-starter", [][]float32{{0, 1}})

	hits, ok := idx.Similar("birth-plan", 2)
	if !ok {
		t.Fatal("Similar returned ok=false for an embedded note")
	}
	if len(hits) != 2 || hits[0].Slug != "hospital-bag" || hits[1].Slug != "sourdough-starter" {
		t.Fatalf("Similar = %+v, want hospital-bag then sourdough-starter", hits)
	}
	for _, h := range hits {
		if h.Slug == "birth-plan" {
			t.Error("Similar returned the note itself")
		}
		if h.Description == "" {
			t.Errorf("hit missing description: %+v", h)
		}
	}

	if _, ok := idx.Similar("health", 5); ok {
		t.Error("Similar returned ok=true for a note with no vectors")
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
