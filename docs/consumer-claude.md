# mnemo on consumer machines

Register the MCP server (per machine, once):

```
claude mcp add --transport http mnemo http://<tailnet-ip>:7920/mcp --header "Authorization: Bearer <token>"
```

Then add the snippet below to the machine's `~/.claude/CLAUDE.md`.

---

## mnemo — personal knowledge vault

My second brain is available through the `mnemo` MCP tools (`vault_*`). It holds durable knowledge: projects, decisions, people, preferences, reference material.

**Reading — lazy, on demand:**
- When a task touches my projects, past decisions, or personal context, check the vault before assuming or asking. Start with `vault_search`; use `vault_index` to browse topic hubs when you don't know what to search for.
- Search returns slugs with one-line descriptions. Only `vault_get` the notes that look relevant; follow `[[wikilinks]]` in a body via further `vault_get` calls when needed.

**Writing — capture raw, never file:**
- When you learn something durable and non-obvious (a decision made, a fact about a project or person, a stated preference), `vault_capture` it. Dump the raw content with enough context to be self-contained; convert relative dates to absolute. A filing agent on the vault machine formats, places, and links it asynchronously — do not try to pick a location or format yourself.
- To correct or extend a specific note you have already read, use `vault_edit` (prefer `append` for additions; pass only the fields that change).
- Never invent slugs. Only use slugs returned by `vault_search`, `vault_index`, or `vault_get`.

Do not store anything derivable from the current repo or conversation — the vault is for knowledge that outlives both.
