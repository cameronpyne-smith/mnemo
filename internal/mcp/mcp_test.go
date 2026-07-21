package mcp

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cameronpyne-smith/mnemo/internal/api"
	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

func testSession(t *testing.T) (*sdk.ClientSession, *store.Store) {
	t.Helper()
	root := t.TempDir()
	if _, err := vault.Init(root); err != nil {
		t.Fatalf("vault.Init: %v", err)
	}
	st, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	if _, err := NewServer(st, nil).Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0.0.0"}, nil)
	sess, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess, st
}

func callTool[T any](t *testing.T, sess *sdk.ClientSession, name string, args any) T {
	t.Helper()
	res, err := sess.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s: tool error: %s", name, contentText(res.Content))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("%s: marshal structured content: %v", name, err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s: unmarshal into %T: %v", name, out, err)
	}
	return out
}

func callToolErr(t *testing.T, sess *sdk.ClientSession, name string, args any) {
	t.Helper()
	res, err := sess.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !res.IsError {
		t.Fatalf("%s(%+v): want tool error, got success", name, args)
	}
}

func contentText(cs []sdk.Content) string {
	var parts []string
	for _, c := range cs {
		if tc, ok := c.(*sdk.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, " ")
}

func saveNote(t *testing.T, st *store.Store, slug, description, body string) {
	t.Helper()
	err := st.Save("test", &vault.Note{
		Slug: slug, Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: description},
		Body:        body,
	})
	if err != nil {
		t.Fatalf("Save %s: %v", slug, err)
	}
}

func TestToolsRegistered(t *testing.T) {
	sess, _ := testSession(t)
	res, err := sess.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	want := []string{"vault_capture", "vault_edit", "vault_get", "vault_index", "vault_links", "vault_search"}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Errorf("tools = %v, want %v", names, want)
	}
}

func TestVaultIndex(t *testing.T) {
	sess, st := testSession(t)
	saveNote(t, st, "birth-plan", "The plan.", "p\n")
	if err := st.AddToHub("test", "health", "birth-plan", "the plan"); err != nil {
		t.Fatalf("AddToHub: %v", err)
	}

	idx := callTool[api.IndexResponse](t, sess, "vault_index", nil)
	if idx.Root.Slug != "root" || len(idx.Hubs) != 1 || idx.Hubs[0].Slug != "health" {
		t.Errorf("index = %+v", idx)
	}
}

func TestVaultGetAndLinks(t *testing.T) {
	sess, st := testSession(t)
	saveNote(t, st, "birth-plan", "The plan.", "See [[hospital-bag]].\n")
	saveNote(t, st, "hospital-bag", "What to pack.", "Packing list.\n")

	note := callTool[api.Note](t, sess, "vault_get", SlugArgs{Slug: "birth-plan"})
	if note.Body != "See [[hospital-bag]].\n" || !slices.Equal(note.Links, []string{"hospital-bag"}) {
		t.Errorf("get = %+v", note)
	}

	links := callTool[api.LinksResponse](t, sess, "vault_links", SlugArgs{Slug: "hospital-bag"})
	if !slices.Equal(links.Backlinks, []string{"birth-plan"}) {
		t.Errorf("links = %+v", links)
	}

	callToolErr(t, sess, "vault_get", SlugArgs{Slug: "nope"})
}

func TestVaultEdit(t *testing.T) {
	sess, st := testSession(t)
	saveNote(t, st, "n", "old", "body\n")

	desc, appended := "new", "more"
	note := callTool[api.Note](t, sess, "vault_edit", EditArgs{Slug: "n", Description: &desc, Append: &appended})
	if note.Description != "new" || note.Body != "body\nmore\n" {
		t.Errorf("edit = %+v", note)
	}

	callToolErr(t, sess, "vault_edit", EditArgs{Slug: "nope", Description: &desc})
}

func TestVaultSearch(t *testing.T) {
	sess, st := testSession(t)
	saveNote(t, st, "birth-plan", "Plan for the birth.", "Details here.\n")
	saveNote(t, st, "grocery-list", "Groceries.", "Milk and eggs.\n")

	res := callTool[api.SearchResponse](t, sess, "vault_search", SearchArgs{Query: "birth"})
	if len(res.Results) != 1 || res.Results[0].Slug != "birth-plan" {
		t.Fatalf("search birth = %+v", res)
	}
	if res.Results[0].Description != "Plan for the birth." || res.Results[0].Score <= 0 {
		t.Errorf("result missing description or score: %+v", res.Results[0])
	}

	res = callTool[api.SearchResponse](t, sess, "vault_search", SearchArgs{Query: "zzz-no-match"})
	if len(res.Results) != 0 {
		t.Errorf("search no-match = %+v", res)
	}

	callToolErr(t, sess, "vault_search", SearchArgs{Query: "   "})
}

func TestVaultCapture(t *testing.T) {
	sess, st := testSession(t)

	res := callTool[api.CaptureResponse](t, sess, "vault_capture", CaptureArgs{Content: "remember the milk", Source: "test"})
	if !strings.HasPrefix(res.Slug, "capture-") {
		t.Fatalf("capture slug = %q, want capture- prefix", res.Slug)
	}

	inbox, err := st.ListInbox()
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if len(inbox) != 1 || inbox[0].Slug != res.Slug {
		t.Fatalf("inbox = %+v, want the captured note", inbox)
	}
	if inbox[0].Body != "remember the milk" && !strings.Contains(inbox[0].Body, "remember the milk") {
		t.Errorf("inbox body = %q, want captured content", inbox[0].Body)
	}

	callToolErr(t, sess, "vault_capture", CaptureArgs{Content: "   "})
}
