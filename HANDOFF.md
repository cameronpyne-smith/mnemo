# mnemo — session handoff (2026-07-15)

Context transfer from the dev-machine Claude Code session to the vault machine.
Read `CLAUDE.md` and `PLAN.md` first — this file only carries what they don't:
current state, verification runbook, and open threads. Delete once absorbed.

## Where things stand

- Phases 0 and 1 are complete and committed (`510ade1` Phase 0, `ea0c8ed` Phase 1,
  `388ddb9` model change). Working tree was clean at handoff.
- All offline tests pass (`go test ./...`). The live/eval tests have **never been
  run against a real ollama** — the dev machine has no GPU. That is the next step.
- Phase 1 was verified end-to-end with the real binary against a scratch vault
  (daemon started with `--no-filing`): capture → search → get (backlinks correct)
  → rename (inbound `[[link]]` rewrite confirmed on disk) → status → daemon-down
  fallback capture. Only the filing agent itself is unverified against a model.

## This machine (vault machine) setup

Prereqs: Go 1.26+, ollama running, models pulled (`qwen3.6`, `qwen3-embedding` —
both already pulled per Cameron).

Run in this order; each step proves a layer the next depends on:

1. **Build + offline tests**
   ```powershell
   go build ./...
   go test ./...
   ```

2. **Live ollama smoke** — proves client wiring, model availability, tool-calling:
   ```powershell
   $env:MNEMO_OLLAMA_TESTS = "http://localhost:11434"
   go test ./internal/ollama -run Live -v
   ```
   First call is slow (24GB model loads into VRAM). If `TestLiveChatToolCall`
   fails, qwen3.6 isn't emitting tool calls via the native API — stop and
   investigate; nothing downstream will work.

3. **Filing eval** — the quality gate (`MNEMO_OLLAMA_TESTS` still set):
   ```powershell
   go test ./internal/agent -run Eval -v
   ```
   Three dumps through the real filing agent against throwaway vaults; prints
   per-check PASS/FAIL and `EVAL SCORE: n/m`. Interpretation:
   - "fact preserved" failures → model drops details → prompt fix
   - "no fragmentation" failures → creates new notes instead of extending →
     prompt fix or try `qwen3.6:27b` (dense A/B candidate)
   - "no tool calls" style aborts → schema/model problem
   Model overrides: `MNEMO_OLLAMA_MODEL`, `MNEMO_OLLAMA_EMBED_MODEL`.

4. **Real vault + daemon**
   ```powershell
   go build -o mnemo.exe ./cmd/mnemo
   .\mnemo.exe init --vault D:\vault    # wherever notes should live
   .\mnemo.exe serve                    # foreground; logs each filing
   ```
   Config lands at `%AppData%\mnemo\config.toml` (`os.UserConfigDir()/mnemo/`).

5. **Use in anger** (second terminal)
   ```powershell
   .\mnemo.exe add "Buy more vanilla protein powder, the MyProtein one"
   .\mnemo.exe status      # inbox drains within ~15s (worker tick) or instantly (wake-on-capture)
   .\mnemo.exe search protein
   .\mnemo.exe get <slug>
   ```
   Then read the vault files by hand — note body, frontmatter `description`,
   hub membership — the human read catches what the eval can't.

## Open threads / known gaps

- **`num_ctx` not wired**: the ollama client sends no `Options`, so ollama's
  small default context applies. If filings truncate or the agent "forgets"
  earlier tool results on long dumps, add `Options{"num_ctx": ...}` to
  `ChatRequest` in `internal/ollama/types.go` + config plumbing. One small change,
  deliberately deferred until symptoms appear.
- **Model choice**: `qwen3.6:35b` (35B-A3B MoE, ~24GB, `:latest`) chosen for
  agentic tool-calling + MoE speed for background filing on the 5090.
  A/B candidate: `qwen3.6:27b` (dense, ~17GB). The eval harness is the arbiter
  for any swap — record the score before and after.
- **Next phase**: Phase 2 (see PLAN.md) — bind to tailnet, MCP server via
  official Go SDK (streamable HTTP), consumer CLAUDE.md snippet, verify
  end-to-end from the work machine. Auth middleware + bearer token already
  exist (built early); Phase 2 is mostly bind config + the MCP wrapper.
- Failed filings intentionally stay in `inbox/` (retried each worker tick);
  successes move to `archive/` with a `filed_into` frontmatter field.
- Nothing beyond the three commits above exists; do not commit/push unless
  Cameron explicitly asks (standing rule, see user CLAUDE.md conventions).
