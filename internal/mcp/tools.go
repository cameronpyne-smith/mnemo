package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cameronpyne-smith/mnemo/internal/api"
	"github.com/cameronpyne-smith/mnemo/internal/store"
)

type SearchArgs struct {
	Query string `json:"query" jsonschema:"what to search for"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of results (default 20)"`
}

type SlugArgs struct {
	Slug string `json:"slug" jsonschema:"the note's slug"`
}

type CaptureArgs struct {
	Content string `json:"content" jsonschema:"raw content to capture"`
	Source  string `json:"source,omitempty" jsonschema:"where the content came from, e.g. claude-code@work"`
}

type EditArgs struct {
	Slug        string    `json:"slug" jsonschema:"the note's slug"`
	Description *string   `json:"description,omitempty" jsonschema:"replacement one-line description"`
	Tags        *[]string `json:"tags,omitempty" jsonschema:"replacement tag list"`
	Body        *string   `json:"body,omitempty" jsonschema:"replacement body"`
	Append      *string   `json:"append,omitempty" jsonschema:"text appended to the end of the body"`
}

func (t *toolServer) index(ctx context.Context, req *sdk.CallToolRequest, _ any) (*sdk.CallToolResult, api.IndexResponse, error) {
	root, hubs, err := t.store.Hubs()
	if err != nil {
		return nil, api.IndexResponse{}, err
	}
	resp := api.IndexResponse{Root: api.FromNote(root, nil, nil), Hubs: make([]api.Note, 0, len(hubs))}
	for _, h := range hubs {
		resp.Hubs = append(resp.Hubs, api.FromNote(h, nil, nil))
	}
	return nil, resp, nil
}

func (t *toolServer) search(ctx context.Context, req *sdk.CallToolRequest, args SearchArgs) (*sdk.CallToolResult, api.SearchResponse, error) {
	hits, err := t.store.Search(args.Query, args.Limit)
	if err != nil {
		return nil, api.SearchResponse{}, err
	}
	return nil, api.FromHits(hits), nil
}

func (t *toolServer) get(ctx context.Context, req *sdk.CallToolRequest, args SlugArgs) (*sdk.CallToolResult, api.Note, error) {
	view, err := t.store.Get(args.Slug)
	if err != nil {
		return nil, api.Note{}, err
	}
	return nil, api.FromNote(view.Note, view.Links, view.Backlinks), nil
}

func (t *toolServer) links(ctx context.Context, req *sdk.CallToolRequest, args SlugArgs) (*sdk.CallToolResult, api.LinksResponse, error) {
	view, err := t.store.Get(args.Slug)
	if err != nil {
		return nil, api.LinksResponse{}, err
	}
	return nil, api.LinksResponse{Slug: view.Note.Slug, Links: view.Links, Backlinks: view.Backlinks}, nil
}

func (t *toolServer) capture(ctx context.Context, req *sdk.CallToolRequest, args CaptureArgs) (*sdk.CallToolResult, api.CaptureResponse, error) {
	slug, err := t.store.Capture(args.Content, args.Source)
	if err != nil {
		return nil, api.CaptureResponse{}, err
	}
	if t.worker != nil {
		t.worker.Wake()
	}
	return nil, api.CaptureResponse{Slug: slug}, nil
}

func (t *toolServer) edit(ctx context.Context, req *sdk.CallToolRequest, args EditArgs) (*sdk.CallToolResult, api.Note, error) {
	err := t.store.EditNote(args.Slug, store.Edit{
		Description: args.Description, Tags: args.Tags, Body: args.Body, Append: args.Append,
	})
	if err != nil {
		return nil, api.Note{}, err
	}
	view, err := t.store.Get(args.Slug)
	if err != nil {
		return nil, api.Note{}, err
	}
	return nil, api.FromNote(view.Note, view.Links, view.Backlinks), nil
}
