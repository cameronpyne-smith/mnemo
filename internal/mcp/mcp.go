package mcp

import (
	"log/slog"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cameronpyne-smith/mnemo/internal/agent"
	"github.com/cameronpyne-smith/mnemo/internal/store"
)

type toolServer struct {
	store  *store.Store
	worker *agent.Worker
}

func NewServer(st *store.Store, worker *agent.Worker) *sdk.Server {
	t := &toolServer{store: st, worker: worker}
	srv := sdk.NewServer(&sdk.Implementation{Name: "mnemo", Version: "0.1.0"}, nil)

	sdk.AddTool(srv, &sdk.Tool{
		Name:        "vault_index",
		Description: "List the vault's root hub and all topic hubs. The cheap entry point: call this first to discover what the vault knows about.",
	}, t.index)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "vault_search",
		Description: "Full-text search across all notes and hubs. Returns slugs with descriptions and relevance scores; follow up with vault_get for full content.",
	}, t.search)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "vault_get",
		Description: "Read one note in full by slug, including its outbound wikilinks and backlinks.",
	}, t.get)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "vault_links",
		Description: "List a note's outbound wikilinks and backlinks without fetching its body.",
	}, t.links)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "vault_capture",
		Description: "Dump raw content into the vault's inbox. A filing agent files it asynchronously; nothing else is required. Use this for any new knowledge worth keeping.",
	}, t.capture)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "vault_edit",
		Description: "Correct or extend an existing note by slug. Only the fields provided change; append adds to the body without replacing it.",
	}, t.edit)

	return srv
}

func Handler(st *store.Store, worker *agent.Worker, log *slog.Logger) http.Handler {
	srv := NewServer(st, worker)
	return sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return srv },
		&sdk.StreamableHTTPOptions{Logger: log},
	)
}
