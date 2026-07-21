# mnemo

Go daemon that manages a second-brain vault of plain markdown notes: captures dumps, files them with a local ollama model, enriches the vault in background "dreamer" passes, and serves it to LLM consumers (CLI, HTTP, MCP) for lazy, on-demand retrieval.

**Read `PLAN.md` before starting work** — it holds the full design decisions, vault conventions, architecture, and phase checklists. Keep its phase checkboxes current as work completes; record any newly agreed design decision in its decisions table.

## Load-bearing invariants

- Markdown files are the ONLY source of truth. Indexes (FTS, vectors, link graph) are in-memory, rebuilt from files, disposable. Never introduce a database or persistent index that the vault cannot rebuild from scratch (the content-hash embedding cache is the one sanctioned derived file).
- The dreamer and filing agent write their findings INTO the markdown (links, hubs, frontmatter). No side stores of knowledge.
- Pure Go, no cgo — Windows is a first-class target. This is why Bleve + brute-force vectors were chosen; do not add cgo deps.
- Vault stays Obsidian-compatible: YAML frontmatter, `[[filename]]` wikilinks, no custom syntax. External tools may edit files at any moment — tolerate foreign formatting, reparse on change.
- Writes are atomic (temp file + rename). A crash must never leave a half-written note. Renames rewrite all inbound links in the same operation.
- Capture must never block on a model: dumps land in `inbox/` instantly; intelligence is async.

## Commands

- Build: `go build ./...`
- Test: `go test ./...`
- Run daemon: `go run ./cmd/mnemo serve`
- Filing eval harness (once built, Phase 1): `go test ./internal/agent -run Eval` — gate for any prompt/model change

## Conventions

- Single module, single binary (`cmd/mnemo`), packages under `internal/` per the layout in PLAN.md.
- Stdlib-first. Approved deps: bleve/v2, yaml.v3, BurntSushi/toml, official MCP Go SDK, cobra (CLI), a Telegram lib (Phase 5). Anything else needs justification.
- The redundancy layer (Phase 2.5) shells out to the system `git` binary via os/exec — assumed installed on the vault machine, verified at daemon startup. Not a Go dependency; do not add a git library.
- Table-driven tests; vault fixtures under `testdata/`. Tests needing a live ollama are gated behind `MNEMO_OLLAMA_TESTS=<base-url>` (skip by default; `MNEMO_OLLAMA_MODEL`/`MNEMO_OLLAMA_EMBED_MODEL` override defaults).
- Errors wrapped with `fmt.Errorf("...: %w", err)`; no panics outside `main`.
- All LLM I/O behind interfaces (`internal/ollama` client is the only implementation for now).
- Model defaults live in config, not code: agent `qwen3.6:35b`, embeddings `qwen3-embedding:8b`.
