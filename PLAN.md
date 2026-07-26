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
| Embedding granularity (2026-07-22) | One embedding per note (chunk = note). Embed text = description + body; scope = `notes/` + `hubs/`, skip `inbox/` and `archive/`. Oversized notes (>~32k chars ≈ 8k tokens; char cap, no tokenizer dep) split on headings internally (paragraphs as fallback), note score = max chunk; results always note-level. Atomicity stays an agent-maintained convention, never enforced. | Notes are agent-written and small by convention; hubs are the pattern for heterogeneous lists (todo items graduate to their own notes when they gain substance). Muddy-centroid risk on mixed notes is half-covered by the FTS side of hybrid search. No lock-in: embeddings are disposable derived data rebuilt from markdown — changing chunking later is a re-embed, not a migration. |
| Hybrid fusion (2026-07-22) | Reciprocal Rank Fusion, k=60, over top-50 candidates from each of FTS and vector; a doc in only one list keeps that list's contribution alone. | Tuned convex combination measurably beats RRF (+2.7–6.9% NDCG across 9 datasets, Bruch et al. TOIS 2023) but requires labeled relevance data to fit α — mnemo has none, and an untuned α is a guess. RRF is the zero-tuning safe default (Qdrant's explicit guidance for exactly this case); k=60 is the near-universal convention (Elasticsearch/Azure/OpenSearch/Vespa), low-sensitivity in [20,100]. Fusion is a pure function over two ranked lists — swappable in isolation. Revisit tuned score fusion once the Phase 6 retrieval log yields implicit relevance data. |
| Capability split by surface (2026-07-26) | External LLM consumers (MCP tools): read, search, capture, append/edit — never organisation (no hub create/delete, no note delete, no rename). Internal agents (filer, dreamer): organise freely, including auto-creating hubs. Human structural ops become CLI commands — create hub (writes the hub and registers it in root.md in one operation), delete note (hard delete; strips the note's entry lines from hubs, reports remaining dead links). No trash mechanism: the git layer is the undo. | Trust boundary enforced by surface, not prompt wording. Routing manual ops through the daemon keeps the store the invariant-keeper (matching hub/root descriptions, fresh in-memory index, single-writer, actor-attributed commits); hand-editing files makes the human the invariant-keeper. |
| Surface reference doc (2026-07-26) | `DOCS.md` at repo root: every exposed operation in one place, grouped by surface — MCP tools, HTTP endpoints (with which auth), CLI commands. PLAN.md keeps rationale; DOCS.md is the living what-is-exposed-where reference, updated in the same change that alters a surface. | The endpoint↔tool table in this file already drifted (`/notes/{slug}/rename` is live but unlisted); auditing the capability split needs one current view. |
| Query instruction prefix (2026-07-22) | Search queries are wrapped with Qwen3-Embedding's stock template `Instruct: Given a web search query, retrieve relevant passages that answer the query\nQuery:{query}` (no space after `Query:` — held fixed; Qwen's own docs are inconsistent on it). Notes are embedded raw; `mnemo similar` compares raw doc vectors on both sides. The instruction lives in config (`embed_query_instruction`; empty = no prefix, for non-instruction-aware models). The daemon builds the string itself — ollama applies no template to `/api/embed` input (nor do vLLM/TEI). Ollama returns L2-normalized vectors: never renormalize; cosine = dot product. Ops: ollama ≥ 0.12.6 and the official library tag (older GGUFs had a pooling/EOS bug costing ~20% accuracy). | Query-only instruction + raw docs is the settled convention since BGE (E5-Mistral, NV-Embed, GTE-Qwen2, Qwen3). Qwen claims 1–5% retrieval loss without it; an independent Qwen3 study (arXiv:2604.06176) measured +8.6% NDCG@5 on clean queries and noise-drop collapsing 17%→0.5% with the prefix. Stock wording over custom: no ablation exists for custom instructions and prompt-sensitivity research (arXiv:2605.22544) shows wording swings can be large — untestable without eval data. Low-regret: docs are embedded raw, so changing the instruction later is a config edit, never a cache re-embed; only a model swap invalidates the cache. Mixed conventions fail silently (miscalibrated cosine, no error), so exactly one code path builds query embeddings and one builds doc embeddings. |

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
| `GET  /search?q=` | `vault_search` | Hybrid FTS + semantic → slugs + descriptions |
| `GET  /notes/{slug}` | `vault_get` | Full note content |
| `GET  /notes/{slug}/similar` | `vault_similar` | Semantically nearest notes (503 while unembedded/disabled) |
| `POST /capture` | `vault_capture` | Dump raw content into inbox (returns capture id) |
| `POST /notes/{slug}/edit` | `vault_edit` | Targeted correction/append to an existing note |
| `GET  /notes/{slug}/links` | `vault_links` | Outbound links + backlinks |
| `GET  /status` | — | Daemon health, index stats, inbox depth, dreamer state |

Writes from external LLMs go through `vault_capture`/`vault_edit`; the filing agent and dreamer keep quality high regardless of who wrote. This table records the designed MCP pairing; the living inventory of every surface (including human-only endpoints and CLI commands) is `DOCS.md`.

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

### Phase 3 — Embeddings + semantic search ✅ (2026-07-25)
- [x] Embedding pipeline: chunk = note (split oversized), content-hash cache on disk, re-embed only changed notes
- [x] Brute-force cosine top-k; hybrid ranking (FTS + vector) in `/search`
- [x] `mnemo similar <slug>` — nearest notes (also the dreamer-linker primitive); also `vault_similar` MCP tool and `GET /notes/{slug}/similar`

### Phase 3.5 — Manual curation surface ✅ (2026-07-26)
- [x] `mnemo hub create <slug> <description...>` — creates the hub and registers it in root.md with a matching description in one operation (`POST /hubs`)
- [x] `mnemo delete <slug>` — hard delete with confirmation (`-y` skips); strips hub entry lines, reports dangling links (`DELETE /notes/{slug}`); root hub undeletable
- [x] `DOCS.md` — living inventory of MCP tools, HTTP endpoints, and CLI commands
- [ ] Admin gate for the structural endpoints (parked: "Admin surface auth" in To be decided later)

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

## To be decided later

Open questions parked here with the option space as understood when parked; none block current phases.

- **Admin surface auth** (noted 2026-07-26). The CLI's structural commands (hub create, note delete — and rename, which already lives on the shared surface) need daemon endpoints: the CLI cannot open the store directly while the daemon holds it (single-writer), and the thin-client topology stands. But MCP shares one listener and one bearer token with the HTTP API, so any MCP consumer that reads its own config can call every endpoint — omitting a tool from the MCP list is cosmetics, not a boundary. Options: (a) second admin bearer token, same listener, admin middleware on the structural routes — works from any machine on the tailnet; current leaning. (b) Structural routes answer only on a loopback listener — no new secret, machine possession is the credential, requires a shell on the vault machine. (c) No endpoints, CLI writes files directly — rejected (single-writer, index staleness, unattributed commits). Interim state (2026-07-26): `POST /hubs` and `DELETE /notes/{slug}` shipped on the shared bearer surface alongside rename — callable by any token holder, protected only by not being MCP tools. Must be resolved before handing the token to anything less trusted than Claude Code.
- **Phone → vault capture while the daemon is up** (noted 2026-07-25). Reachable today over the tailnet, so this is a front-end choice, not architecture: iOS Shortcut share-sheet → `POST /capture` with the bearer token (Tailscale on-demand VPN), the Telegram gateway (P5), or SSH + `mnemo add` from a terminal app. No decision needed until P5 makes the comparison real.
- **Capture while every machine is off, vault included** (noted 2026-07-25). Options considered:
  - *Phone-side queue, flush when the daemon returns* — captures accumulate on the phone (Drafts action or a Shortcuts pair: share-sheet append + flush automation), POST to `/capture` when reachable. Current leaning: plaintext stays on owned hardware, zero new infrastructure, and vault uptime is known and user-controlled, so a manual-ish flush is acceptable.
  - *Telegram as store-and-forward* — zero extra hardware, but the Bot API drops unfetched updates after ~24h (daemon down for a weekend = silent loss) and bot chats are not E2E-encrypted, so capture text sits in plaintext on Telegram's servers — an exception to the redundancy decision's plaintext-never-leaves-owned-hardware rule. Acceptable per-message for low-sensitivity captures at most.
  - *Raspberry Pi as always-on queue relay* (several on hand) — accepts captures 24/7 and forwards to `/capture` when the daemon returns. Pis are too small to host the vault itself and notes must stay on the vault machine, so this is a relay only. Leaning against: an extra infrastructure hop for a gap the phone-side queue already covers.

## Engineering constraints

- Pure Go only — no cgo (Windows is a first-class target). This constraint drove the index design; do not add cgo dependencies.
- Dependencies: stdlib-first; approved so far: bleve/v2, yaml.v3, official MCP Go SDK, a Telegram bot lib (P5). Justify anything else.
- The vault must always be valid: atomic writes (temp file + rename), never leave a note half-written. Assume Obsidian or a human may edit files at any time — reparse on change, tolerate foreign formatting.
- All model I/O behind interfaces; the eval harness is the arbiter for prompt/model changes.
- Windows + Linux supported; paths via `filepath` everywhere.
