package index

import (
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"

	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

type Hit struct {
	Slug        string
	Folder      string
	Description string
	Score       float64
}

type Index struct {
	mu       sync.RWMutex
	fts      bleve.Index
	folders  map[string]string
	outbound map[string][]string
	inbound  map[string]map[string]bool
	vectors  map[string][]float32
}

func New() (*Index, error) {
	fts, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		return nil, fmt.Errorf("creating search index: %w", err)
	}
	return &Index{
		fts:      fts,
		folders:  make(map[string]string),
		outbound: make(map[string][]string),
		inbound:  make(map[string]map[string]bool),
		vectors:  make(map[string][]float32),
	}, nil
}

func Build(v *vault.Vault) (*Index, error) {
	idx, err := New()
	if err != nil {
		return nil, err
	}
	notes, err := v.List(vault.FolderNotes, vault.FolderHubs)
	if err != nil {
		return nil, fmt.Errorf("building index: %w", err)
	}
	for _, n := range notes {
		if err := idx.IndexNote(n); err != nil {
			return nil, err
		}
	}
	return idx, nil
}

func (idx *Index) IndexNote(n *vault.Note) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	doc := map[string]any{
		"slug":        n.Slug,
		"folder":      n.Folder,
		"type":        n.Type(),
		"description": n.Frontmatter.Description,
		"tags":        n.Frontmatter.Tags,
		"body":        n.Body,
	}
	if err := idx.fts.Index(n.Slug, doc); err != nil {
		return fmt.Errorf("indexing note %s: %w", n.Slug, err)
	}

	idx.folders[n.Slug] = n.Folder
	idx.removeEdgesLocked(n.Slug)
	links := n.Links()
	idx.outbound[n.Slug] = links
	for _, target := range links {
		if idx.inbound[target] == nil {
			idx.inbound[target] = make(map[string]bool)
		}
		idx.inbound[target][n.Slug] = true
	}
	return nil
}

func (idx *Index) RemoveNote(slug string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if err := idx.fts.Delete(slug); err != nil {
		return fmt.Errorf("removing note %s from index: %w", slug, err)
	}
	delete(idx.folders, slug)
	delete(idx.vectors, slug)
	idx.removeEdgesLocked(slug)
	return nil
}

func (idx *Index) SetVector(slug string, vec []float32) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.vectors[slug] = vec
}

func (idx *Index) RemoveVector(slug string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.vectors, slug)
}

func (idx *Index) Vectors() []DocVector {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]DocVector, 0, len(idx.vectors))
	for _, slug := range slices.Sorted(maps.Keys(idx.vectors)) {
		out = append(out, DocVector{Slug: slug, Vec: idx.vectors[slug]})
	}
	return out
}

func (idx *Index) removeEdgesLocked(slug string) {
	for _, target := range idx.outbound[slug] {
		delete(idx.inbound[target], slug)
		if len(idx.inbound[target]) == 0 {
			delete(idx.inbound, target)
		}
	}
	delete(idx.outbound, slug)
}

func (idx *Index) Search(q string, limit int) ([]Hit, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}
	fields := []struct {
		name  string
		boost float64
	}{
		{"description", 3},
		{"slug", 2.5},
		{"tags", 2},
		{"body", 1},
	}
	var queries []query.Query
	for _, f := range fields {
		mq := bleve.NewMatchQuery(q)
		mq.SetField(f.name)
		mq.SetBoost(f.boost)
		queries = append(queries, mq)
	}
	req := bleve.NewSearchRequestOptions(bleve.NewDisjunctionQuery(queries...), limit, 0, false)
	req.Fields = []string{"description", "folder"}

	res, err := idx.fts.Search(req)
	if err != nil {
		return nil, fmt.Errorf("searching for %q: %w", q, err)
	}
	hits := make([]Hit, 0, len(res.Hits))
	for _, h := range res.Hits {
		hit := Hit{Slug: h.ID, Score: h.Score}
		if d, ok := h.Fields["description"].(string); ok {
			hit.Description = d
		}
		if f, ok := h.Fields["folder"].(string); ok {
			hit.Folder = f
		}
		hits = append(hits, hit)
	}
	return hits, nil
}

func (idx *Index) Backlinks(slug string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return slices.Sorted(maps.Keys(idx.inbound[slug]))
}

func (idx *Index) Outbound(slug string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return append([]string(nil), idx.outbound[slug]...)
}

func (idx *Index) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.folders)
}
