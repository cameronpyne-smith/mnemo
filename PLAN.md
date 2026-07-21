# mnemo — Project Plan

A second-brain agent. mnemo owns a vault of plain markdown notes, files whatever gets dumped into it, and continuously enriches the vault in the background — rewriting, summarising, linking, organising — so that any LLM (mnemo itself, Claude Code on another machine, anything speaking MCP) can lazily discover and read exactly the notes it needs.

## Design decisions (resolved 2026-07-14)

| Decision | Choice | Why |
|---|---|---|
| Intelligence | Local ollama, native API (`/api/chat`, `/api/embed`) | Runs on the vault machine (Threadripper PRO 9965WX, RTX 5090 32GB). Native API chosen over OpenAI-compat layer. |
| Topology | One daemon on the vault machine exposing an HTTP API; CLI and MCP server are thin clients of the same core | Vault machine ≠ consumer machines. One core, several frontends. |
| Source of truth | Markdown files only. Obsidian-compatible: YAML frontmatter + `[[wikilinks]]` | Vault must open cleanly in Obsidian, but design is agent-first. |
| Identity | Filename = human kebab slug; links resolve by filename; renames rewrite all inbound links atomically; **no separate IDs** | Matches Obsidian/Claude Code memory. mnemo owns the link graph, so rename-rewrite is cheap. |
| Vault layout | Flat semantics. Five mechanical folders: `notes/`, `hubs/`, `inbox/`, `attachments/`, `archive/` | Folders encode pipeline state / file type, never topic. Meaning lives in links, hubs, frontmatter. Removes folder-choice from the filing model. |
| Lazy loading | Hub notes (Maps of Content): root hub → topic hubs → notes, each entry `[[link]] — one-line description`; every note carries a `description:` frontmatter field | Same pattern as skills / Claude Code MEMORY.md. Agents drill down; nothing loads wholesale. |
| Index | **No database.** In-memory indexes rebuilt from files at startup: Bleve (FTS), normalized float32 vectors + brute-force cosine, adjacency-map link graph. One on-disk embedding cache keyed by content hash | ~4ms search at 10k notes; matches what Smart Connections/Obsidian Copilot ship. Research: agent-memory field is converging on boring storage; nobody uses a graph DB for wikilink graphs. Index code behind an interface so SQLite could slot in later. |
| Ingest | Capture now, file async. Dumps land in `inbox/` instantly (durable, never blocked by the model); the filing agent processes the inbox: pick/create target note, rewrite/merge, frontmatter, links | Capture must never fail because a model is slow or down. |
| Dreamer | Background subsystem inside the daemon (hippo-inspired). Passes write results **into the markdown** — never into a side database | The wikilink structure is the only graph. No second source of truth. |
| Remote access | Daemon binds to the tailnet; static bearer token | Zero internet-exposed ports, encrypted transport, works away from home. |
| Models | Configurable. Defaults: agent `qwen3.6:35b` (35B-A3B MoE; A/B candidate `qwen3.6:27b` dense), embeddings `qwen3-embedding:8b` | Qwen3.6 (2026-04) targets agentic tool-calling; MoE with 3B active = fast background filing on the 5090. Eval harness makes swaps measurable. |
| Learning loop (final phase) | (a) Self-tuning conventions: mnemo maintains its own instructions note in the vault and records lessons when corrected. (b) Usage-driven salience: retrieval log; hot notes rank higher and get dreamer attention; cold notes decay toward `archive/` | Chosen over filing metrics and skill acquisition. |
| Starting state | Empty vault | No importers needed. |
| MCP topology | MCP server wraps `store.Store` in-process, mounted at `/mcp` on the same listener behind the same bearer auth — not a proxy over the HTTP API | One core, two front doors; no double serialization, no localhost hop, one port to expose on the tailnet. |
| Redundancy (2026-07-21) | Vault is a git repo. Daemon auto-commits every mutation via the system git CLI (os/exec, no Go deps) and pushes async to an append-only bare remote on an external drive (BitLocker To Go); personal-machine tailnet remote deferred. No snapshot tooling, no third-party hosts. | Point-in-time history defends against bad LLM writes — mirrors/sync replicate the damage. obsidian-git precedent: git fits live vaults incl. foreign edits. System git over go-git: zero deps, reference implementation, and real `gc`/`maintenance` — go-git can't repack, so per-mutation commits bloat unboundedly; git presence verified at startup. `receive.denyNonFastForwards` + `receive.denyDeletes` make remotes append-only even from a compromised vault machine. Plaintext never leaves owned hardware. Git-everywhere beats nightly snapshots for granularity (per-edit revert, seconds of lag); the uncovered scenario is simultaneous house loss of all copies — offsite decision deferred. |

## Vault conventions

```
vault/
  hubs/          # Maps of Content, including root.md — the entry point
  notes/         # all knowledge notes (flat)
  inbox/         # raw captures awaiting filing (one file per dump)
  attachments/   # binaries; referenced from notes, never indexed as notes
  archive/       # retired notes; excluded from default search
```

Note frontmatter:

```yaml
---
description: One line saying what is in this note — used for lazy discovery.
tags: [health, project-x]        # optional
type: note | hub                  # default note
created: 2026-07-14
updated: 2026-07-14
---
```

Rules:
- Filenames: kebab-case slugs, unique across the vault.
- Every note MUST have a `description` and appear in at least one hub.
- Links: `[[filename]]` (no path, no extension). Renames rewrite all inbound links in the same operation.
- Hubs are ordinary notes (`type: hub`) whose body is a curated list of `[[link]] — description` lines. `hubs/root.md` links to every hub.
- Attachments referenced by standard markdown links to `attachments/`.
- Archiving = move to `archive/` (links preserved, excluded from default search).

## Architecture

```
                    ┌────────────────────── mnemo daemon (vault machine) ─────────────────────┐
 phone (Telegram)──►│ gateway (P6)                                                            │
                    │      │                ┌──────────┐      ┌───────────────────────────┐   │
 CLI (any machine)─►│ HTTP API ──────────►  │  core    │ ───► │ vault (markdown on disk)  │   │
                    │      │                │ library  │      └───────────────────────────┘   │
 Claude Code ──────►│ MCP (streamable HTTP) │          │      ┌───────────────────────────┐   │
                    │                       │          │ ───► │ in-memory indexes         │   │
                    │  filing agent ◄─── inbox watch   │      │ FTS · vectors · linkgraph │   │
                    │  dreamer      ◄─── scheduler     │      └───────────────────────────┘   │
                    │        └── ollama (native API)   │       embedding cache (disk)         │
                    └─────────────────────────────────────────────────────────────────────────┘
```

Package layout (single module, single binary):

```
cmd/mnemo/          # CLI: serve, add, search, get, status, ...
internal/config/    # TOML config: vault path, bind addr, token, model names
internal/vault/     # note model, frontmatter, wikilink parsing, read/write/rename
internal/index/     # Index interface; bleve FTS, vector store, link graph; rebuild
internal/ollama/    # native API client: chat w/ tools, embeddings
internal/agent/     # agentic loop + filing agent prompts/tools
internal/dreamer/   # scheduler + passes (linker, consolidator, hubs, gardener)
internal/server/    # HTTP API + auth middleware
internal/mcp/       # MCP server (streamable HTTP) over the same core
internal/gateway/   # Telegram (P6)
```

HTTP API (also the MCP tool surface):

| Endpoint | MCP tool | Purpose |
|---|---|---|
| `GET  /index` | `vault_index` | Root hub + hub listing — the cheap entry point |
| `GET  /search?q=` | `vault_search` | Hybrid FTS(+semantic from P4) → slugs + descriptions |
| `GET  /notes/{slug}` | `vault_get` | Full note content |
| `POST /capture` | `vault_capture` | Dump raw content into inbox (returns capture id) |
| `POST /notes/{slug}/edit` | `vault_edit` | Targeted correction/append to an existing note |
| `GET  /notes/{slug}/links` | `vault_links` | Outbound links + backlinks |
| `GET  /status` | — | Daemon health, index stats, inbox depth, dreamer state |

Writes from external LLMs go through `vault_capture`/`vault_edit`; the filing agent and dreamer keep quality high regardless of who wrote.

## Phases

Each phase ends runnable and used-in-anger before the next starts.

### Phase 0 — Foundations ✅ (2026-07-14)
- [x] TOML config + `mnemo` CLI skeleton (cobra; config at `os.UserConfigDir()/mnemo/config.toml`, `--config` override)
- [x] `internal/vault`: note read/write, frontmatter codec (foreign fields preserved via inline map, CRLF tolerated), wikilink extraction, slug rules, atomic file writes
- [x] `internal/ollama`: chat (with tool calling) + embed against the native API; live smoke tests gated behind `MNEMO_OLLAMA_TESTS`
- [x] Vault bootstrap: `mnemo init` creates folder skeleton + root hub (idempotent, never overwrites)
- [x] Test fixtures: a small `testdata/` vault including foreign-formatted notes

### Phase 1 — Vault engine + CLI (v1 milestone) ✅ (2026-07-14)
- [x] In-memory FTS (Bleve) + link graph; full rebuild on daemon start; incremental update on writes (`internal/index`, `internal/store`)
- [x] Daemon: `mnemo serve` — HTTP API (localhost first), inbox worker (15s scan + wake-on-capture); `--no-filing` to disable the agent
- [x] Capture path: `mnemo add "..."` / stdin / `-f file` → inbox; falls back to writing the inbox file directly when the daemon is down
- [x] Filing agent: agentic loop (search_notes, read_note, write_note, add_to_hub, finish); failed filings stay in inbox; processed captures move to archive/ with `filed_into`
- [x] `mnemo search`, `mnemo get`, `mnemo status`, `mnemo rename` against the API; also `/index`, `/notes/{slug}/links`, `/notes/{slug}/edit` endpoints and bearer-token auth (early, ahead of Phase 2)
- [x] Rename-with-rewrite operation (aliases and heading anchors included)
- [x] Filing eval harness: `MNEMO_OLLAMA_TESTS=<url> go test ./internal/agent -run Eval -v` — scores capture→file outcomes (facts preserved, inbox drained, hub reachability, no fragmentation)

### Phase 2 — Remote access + MCP (Claude Code integration)
- [x] Bearer-token auth middleware (landed early in P1); bind to tailnet address = config `bind` value, no code
- [x] MCP server (official Go SDK, streamable HTTP) exposing the tool surface above — all six handlers done and tested
- [x] Snippet for consumer machines' CLAUDE.md documenting when/how to use the vault (`docs/consumer-claude.md`)
- [x] Verify end-to-end from work machine: discover → read → capture → correct (verified 2026-07-19 over tailnet MCP; capture filed by agent, correction appended via vault_edit)

### Phase 2.5 — Redundancy (must land before the dreamer gets write access)
- [x] Vault is a git repo; daemon auto-commits its own mutations with actor-tagged messages (system git CLI via os/exec — startup check fails loud if git missing; `git gc --auto` once at startup, since commit-only repos never trigger git's auto-packing); external edits are the user's to commit — pathspec-limited commits keep them out of daemon history; baseline commit only when the repo is first created; embedding cache + write temp files gitignored
- [x] Async push queue: push all remotes after each commit; retry with backoff (30s→15m) while a remote is unreachable; full catch-up on reconnect; per-remote lag (commits ahead) surfaced in `mnemo status` + `/status`
- [ ] Bare repo on external drive (BitLocker To Go); `receive.denyNonFastForwards` + `receive.denyDeletes` — `mnemo backup init <path>` creates and registers it; run on the vault machine
- [ ] (deferred) Bare repo on personal machine — OpenSSH on tailnet only, key-only auth, same receive protections; revisit once the external-drive remote proves out
- [ ] BitLocker on the vault machine volume holding the vault
- [ ] Restore drill documented and performed: single-note rollback via git, full-vault clone from each remote; periodic `git fsck` on remotes
- [ ] Decide offsite story for the house-loss scenario (work-machine remote / encrypted bundle drop / accept risk)

### Phase 3 — Embeddings + semantic search
- [ ] Embedding pipeline: chunk = note (split oversized), content-hash cache on disk, re-embed only changed notes
- [ ] Brute-force cosine top-k; hybrid ranking (FTS + vector) in `/search`
- [ ] `mnemo similar <slug>` — nearest notes (also the dreamer-linker primitive)

### Phase 4 — Dreamer
- [ ] Scheduler: idle-time passes, per-pass budgets, `mnemo dream` to trigger manually, report of actions taken
- [ ] Linker: vector-similar candidate pairs → LLM judges → writes wikilinks with context
- [ ] Hub maintenance: new/orphan notes added to hubs, stale descriptions rewritten, oversized hubs split
- [ ] Consolidator: detect duplicates/overlaps → merge or cross-link; contradiction flagged in-note
- [ ] Gardener: frontmatter validation/repair, broken links, inbox stragglers
- [ ] Every dreamer action logged to a journal note in the vault

### Phase 5 — Telegram gateway
- [ ] Bot long-polling into `/capture`; text, forwarded messages, images→attachments, voice notes (transcription via whisper model on ollama or skip v1)
- [ ] Conversational queries: messages routed to the agent loop with vault tools; answers back in chat
- [ ] Filing confirmations ("filed under [[x]], linked to [[y]]") + correction replies

### Phase 6 — Learning loop
- [ ] `hubs/mnemo-conventions.md`: mnemo's own operating instructions, loaded into filing/dreamer prompts; corrections append lessons
- [ ] Retrieval log in the daemon; salience score per note
- [ ] Salience feeds search ranking and dreamer attention; cold notes proposed for `archive/`

## Engineering constraints

- Pure Go only — no cgo (Windows is a first-class target). This constraint drove the index design; do not add cgo dependencies.
- Dependencies: stdlib-first; approved so far: bleve/v2, yaml.v3, official MCP Go SDK, a Telegram bot lib (P5). Justify anything else.
- The vault must always be valid: atomic writes (temp file + rename), never leave a note half-written. Assume Obsidian or a human may edit files at any time — reparse on change, tolerate foreign formatting.
- All model I/O behind interfaces; the eval harness is the arbiter for prompt/model changes.
- Windows + Linux supported; paths via `filepath` everywhere.
