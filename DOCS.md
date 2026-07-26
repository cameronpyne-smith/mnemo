# mnemo — exposed surfaces

Living inventory of every operation mnemo exposes, grouped by surface. Update this file in the same change that adds, removes, or re-guards an operation. Design rationale lives in `PLAN.md`.

The capability split (PLAN.md decisions, 2026-07-26): external LLM consumers (MCP) read, search, capture, and append/edit — never organisation. Internal agents (filer, dreamer) organise freely. Structural and destructive operations are human-only, via the CLI.

## MCP tools

Model-facing surface, mounted at `/mcp` on the daemon listener behind the bearer token.

| Tool | Purpose |
|---|---|
| `vault_index` | Root hub + hub listing — the cheap entry point |
| `vault_search` | Hybrid FTS + semantic search → slugs + descriptions |
| `vault_similar` | Semantically nearest notes to a note |
| `vault_get` | Full note content |
| `vault_links` | Outbound links + backlinks of a note |
| `vault_capture` | Dump raw content into the inbox for async filing |
| `vault_edit` | Targeted correction/append to an existing note |

## HTTP API

All endpoints require the bearer token (`Authorization: Bearer <token>`); the daemon binds to the tailnet.

| Endpoint | MCP tool | Purpose |
|---|---|---|
| `GET  /index` | `vault_index` | Root hub + hub listing |
| `GET  /search?q=` | `vault_search` | Hybrid search |
| `GET  /notes/{slug}` | `vault_get` | Full note content |
| `GET  /notes/{slug}/links` | `vault_links` | Outbound links + backlinks |
| `GET  /notes/{slug}/similar` | `vault_similar` | Nearest notes (503 while unembedded/disabled) |
| `POST /notes/{slug}/edit` | `vault_edit` | Edit description/tags/body, or append |
| `POST /notes/{slug}/rename` | — | Rename, rewriting all inbound links |
| `DELETE /notes/{slug}` | — | Hard delete; strips hub entry lines, reports dangling links |
| `POST /hubs` | — | Create a hub and register it in the root hub |
| `POST /capture` | `vault_capture` | Dump raw content into the inbox |
| `GET  /status` | — | Daemon health, index stats, filing/git/embedding state |
| `/mcp` | — | MCP server mount (streamable HTTP) |

**Caveat — one token guards everything.** The endpoints without an MCP tool (`rename`, `DELETE /notes/{slug}`, `POST /hubs`) are intended as human-only, but today they sit behind the same bearer token as the MCP mount, so any MCP consumer that reads its own config can call them. Not a security boundary yet — see "Admin surface auth" under *To be decided later* in `PLAN.md`.

## CLI

`mnemo` talks to the daemon's HTTP API using `bind` + `token` from the config file, except where noted.

| Command | Purpose |
|---|---|
| `mnemo init --vault <path>` | Create the vault skeleton + root hub + config file (local, no daemon) |
| `mnemo serve` | Run the daemon: HTTP API, MCP, filing worker, git sync, embeddings |
| `mnemo add [text] [-f file]` | Capture into the inbox; falls back to writing the inbox file directly if the daemon is down |
| `mnemo search <query>` | Hybrid search |
| `mnemo similar <slug>` | Semantically nearest notes |
| `mnemo get <slug>` | Print a note with tags and backlinks |
| `mnemo status` | Daemon + vault + filing + git + embedding status |
| `mnemo rename <slug> <new-slug>` | Rename, rewriting all inbound links |
| `mnemo hub create <slug> <description...>` | Create a hub, registered in root.md with a matching description |
| `mnemo delete <slug>` | Hard delete with confirmation (`-y` skips); strips hub entries, reports dangling links |
| `mnemo backup init <path>` | Create + register an append-only bare git remote (run on the vault machine) |
