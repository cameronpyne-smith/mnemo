package api

import (
	"github.com/cameronpyne-smith/mnemo/internal/index"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

func FromNote(n *vault.Note, links, backlinks []string) Note {
	return Note{
		Slug:        n.Slug,
		Folder:      n.Folder,
		Description: n.Frontmatter.Description,
		Tags:        n.Frontmatter.Tags,
		Type:        n.Type(),
		Created:     n.Frontmatter.Created,
		Updated:     n.Frontmatter.Updated,
		Body:        n.Body,
		Links:       links,
		Backlinks:   backlinks,
	}
}

func FromHits(hits []index.Hit) SearchResponse {
	resp := SearchResponse{Results: make([]SearchResult, 0, len(hits))}
	for _, h := range hits {
		resp.Results = append(resp.Results, SearchResult{
			Slug: h.Slug, Folder: h.Folder, Description: h.Description, Score: h.Score,
		})
	}
	return resp
}
