package agent

import (
	"encoding/json"

	"github.com/cameronpyne-smith/mnemo/internal/ollama"
)

var toolDefs = []ollama.Tool{
	ollama.NewTool("search_notes", "Search the vault. Returns matching notes as wikilink, folder, and description lines.", json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Keywords to search for."}
		},
		"required": ["query"]
	}`)),
	ollama.NewTool("read_note", "Read a note's full content, tags, and backlinks.", json.RawMessage(`{
		"type": "object",
		"properties": {
			"slug": {"type": "string", "description": "The note's slug."}
		},
		"required": ["slug"]
	}`)),
	ollama.NewTool("write_note", "Create or fully replace a note in the vault. Pass the complete body.", json.RawMessage(`{
		"type": "object",
		"properties": {
			"slug": {"type": "string", "description": "Kebab-case slug naming the note."},
			"description": {"type": "string", "description": "One line saying what is in the note."},
			"tags": {"type": "array", "items": {"type": "string"}, "description": "Short lowercase topic tags."},
			"body": {"type": "string", "description": "Full markdown body, including [[wikilinks]] to related notes."}
		},
		"required": ["slug", "description", "body"]
	}`)),
	ollama.NewTool("add_to_hub", "Add a note to a hub so it is discoverable. Creates the hub if it does not exist.", json.RawMessage(`{
		"type": "object",
		"properties": {
			"hub": {"type": "string", "description": "Hub slug, e.g. health or project-x."},
			"note": {"type": "string", "description": "Slug of the note to add."},
			"description": {"type": "string", "description": "One-line description shown next to the link."}
		},
		"required": ["hub", "note", "description"]
	}`)),
	ollama.NewTool("finish", "Declare filing complete. Call exactly once, last.", json.RawMessage(`{
		"type": "object",
		"properties": {
			"notes": {"type": "array", "items": {"type": "string"}, "description": "Slugs of the notes the capture was filed into."},
			"summary": {"type": "string", "description": "One line describing what was filed where."}
		},
		"required": ["notes", "summary"]
	}`)),
}
