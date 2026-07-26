package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	if _, err := vault.Init(root); err != nil {
		t.Fatalf("vault.Init: %v", err)
	}
	s, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func mustSave(t *testing.T, s *Store, n *vault.Note) {
	t.Helper()
	if err := s.Save("test", n); err != nil {
		t.Fatalf("Save(%s): %v", n.Slug, err)
	}
}

func TestSaveSetsDatesAndIndexes(t *testing.T) {
	s := testStore(t)
	mustSave(t, s, &vault.Note{
		Slug: "birth-plan", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "Plan for the birth."},
		Body:        "Details.\n",
	})

	view, err := s.Get("birth-plan")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if view.Note.Frontmatter.Created == "" || view.Note.Frontmatter.Updated == "" {
		t.Errorf("dates not set: %+v", view.Note.Frontmatter)
	}
	hits, err := s.Search(t.Context(), "birth", 10)
	if err != nil || len(hits) == 0 {
		t.Fatalf("Search after Save: hits=%v err=%v", hits, err)
	}
}

func TestCreateHub(t *testing.T) {
	s := testStore(t)
	hub, err := s.CreateHub("test", "recipes", "Cooking recipes and meal ideas.")
	if err != nil {
		t.Fatalf("CreateHub: %v", err)
	}
	if hub.Type() != vault.TypeHub {
		t.Errorf("type = %s, want hub", hub.Type())
	}

	root, hubs, err := s.Hubs()
	if err != nil {
		t.Fatalf("Hubs: %v", err)
	}
	if len(hubs) != 1 || hubs[0].Slug != "recipes" {
		t.Errorf("hubs = %v, want [recipes]", hubs)
	}
	if !strings.Contains(root.Body, "- [[recipes]] — Cooking recipes and meal ideas.") {
		t.Errorf("root not registered with matching description:\n%s", root.Body)
	}
	if strings.Contains(root.Body, "No hubs yet.") {
		t.Errorf("placeholder not removed:\n%s", root.Body)
	}

	for name, args := range map[string][2]string{
		"duplicate":         {"recipes", "Again."},
		"invalid slug":      {"Bad Slug", "d"},
		"empty description": {"empty-desc", "  "},
	} {
		if _, err := s.CreateHub("test", args[0], args[1]); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestDeleteNote(t *testing.T) {
	s := testStore(t)
	mustSave(t, s, &vault.Note{
		Slug: "old-recipe", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "A recipe."}, Body: "Steps.\n",
	})
	mustSave(t, s, &vault.Note{
		Slug: "cooking-notes", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "Notes."},
		Body:        "See [[old-recipe]] for details.\n",
	})
	if err := s.AddToHub("test", "recipes", "old-recipe", "a recipe"); err != nil {
		t.Fatalf("AddToHub: %v", err)
	}

	res, err := s.DeleteNote("test", "old-recipe")
	if err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	if res.Folder != vault.FolderNotes {
		t.Errorf("folder = %s, want notes", res.Folder)
	}
	if len(res.RemovedFromHubs) != 1 || res.RemovedFromHubs[0] != "recipes" {
		t.Errorf("RemovedFromHubs = %v, want [recipes]", res.RemovedFromHubs)
	}
	if len(res.DanglingLinks) != 1 || res.DanglingLinks[0] != "cooking-notes" {
		t.Errorf("DanglingLinks = %v, want [cooking-notes]", res.DanglingLinks)
	}

	if _, err := s.Get("old-recipe"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete: err = %v, want ErrNotFound", err)
	}
	hub, err := s.Get("recipes")
	if err != nil {
		t.Fatalf("Get hub: %v", err)
	}
	if strings.Contains(hub.Note.Body, "old-recipe") {
		t.Errorf("hub entry not stripped:\n%s", hub.Note.Body)
	}
	hits, err := s.Search(t.Context(), "recipe", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, h := range hits {
		if h.Slug == "old-recipe" {
			t.Errorf("deleted note still indexed: %v", hits)
		}
	}
}

func TestDeleteHubUnregistersFromRoot(t *testing.T) {
	s := testStore(t)
	if _, err := s.CreateHub("test", "recipes", "Cooking."); err != nil {
		t.Fatalf("CreateHub: %v", err)
	}

	res, err := s.DeleteNote("test", "recipes")
	if err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	if len(res.RemovedFromHubs) != 1 || res.RemovedFromHubs[0] != "root" {
		t.Errorf("RemovedFromHubs = %v, want [root]", res.RemovedFromHubs)
	}
	root, hubs, err := s.Hubs()
	if err != nil {
		t.Fatalf("Hubs: %v", err)
	}
	if len(hubs) != 0 || strings.Contains(root.Body, "recipes") {
		t.Errorf("hub not fully removed: hubs=%v root:\n%s", hubs, root.Body)
	}
}

func TestDeleteRejectsRootAndMissing(t *testing.T) {
	s := testStore(t)
	if _, err := s.DeleteNote("test", "root"); !errors.Is(err, ErrInvalid) {
		t.Errorf("delete root: err = %v, want ErrInvalid", err)
	}
	if _, err := s.DeleteNote("test", "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing: err = %v, want ErrNotFound", err)
	}
}

func TestStripEntryLines(t *testing.T) {
	body := "# hub\n\n- [[target]] — the entry\n* [[target|alias]] bullet\n- [[target-not]] — keep\n- [[other]] — mentions [[target]] in prose\nSee [[target]].\n"
	got, removed := stripEntryLines(body, "target")
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}
	want := "# hub\n\n- [[target-not]] — keep\n- [[other]] — mentions [[target]] in prose\nSee [[target]].\n"
	if got != want {
		t.Errorf("stripEntryLines:\ngot  %q\nwant %q", got, want)
	}
}

func TestSearchRejectsBlankQuery(t *testing.T) {
	s := testStore(t)
	for _, q := range []string{"", "   "} {
		if _, err := s.Search(t.Context(), q, 10); !errors.Is(err, ErrInvalid) {
			t.Errorf("Search(%q): err = %v, want ErrInvalid", q, err)
		}
	}
}

func TestCaptureAndArchive(t *testing.T) {
	s := testStore(t)
	slug, err := s.Capture("test", "Buy vanilla protein powder", "test")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if !strings.HasPrefix(slug, "capture-") {
		t.Errorf("slug = %s, want capture- prefix", slug)
	}

	inbox, err := s.ListInbox()
	if err != nil || len(inbox) != 1 {
		t.Fatalf("ListInbox = %v, %v; want one item", inbox, err)
	}

	if err := s.ArchiveCapture("test", slug, []string{"shopping"}); err != nil {
		t.Fatalf("ArchiveCapture: %v", err)
	}
	inbox, err = s.ListInbox()
	if err != nil || len(inbox) != 0 {
		t.Fatalf("inbox after archive = %v, %v; want empty", inbox, err)
	}
	view, err := s.Get(slug)
	if err != nil {
		t.Fatalf("Get archived: %v", err)
	}
	if view.Note.Folder != vault.FolderArchive {
		t.Errorf("folder = %s, want archive", view.Note.Folder)
	}
	filed, ok := view.Note.Frontmatter.Extra["filed_into"].([]any)
	if !ok || len(filed) != 1 || filed[0] != "shopping" {
		t.Errorf("filed_into = %v", view.Note.Frontmatter.Extra["filed_into"])
	}
}

func TestCaptureRejectsEmpty(t *testing.T) {
	s := testStore(t)
	if _, err := s.Capture("test", "  \n", "test"); err == nil {
		t.Fatal("Capture accepted empty content")
	}
}

func TestRenameRewritesBacklinks(t *testing.T) {
	s := testStore(t)
	mustSave(t, s, &vault.Note{
		Slug: "birth-plan", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "The plan."}, Body: "Plan.\n",
	})
	mustSave(t, s, &vault.Note{
		Slug: "hospital-bag", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "The bag."},
		Body:        "See [[birth-plan]], [[birth-plan|the plan]] and [[birth-plan#risks]].\n",
	})

	if err := s.Rename("test", "birth-plan", "labour-plan"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if _, err := s.Get("birth-plan"); err == nil {
		t.Error("old slug still resolves")
	}
	view, err := s.Get("labour-plan")
	if err != nil {
		t.Fatalf("Get new slug: %v", err)
	}
	if len(view.Backlinks) != 1 || view.Backlinks[0] != "hospital-bag" {
		t.Errorf("backlinks = %v, want [hospital-bag]", view.Backlinks)
	}

	bag, err := s.Get("hospital-bag")
	if err != nil {
		t.Fatalf("Get hospital-bag: %v", err)
	}
	want := "See [[labour-plan]], [[labour-plan|the plan]] and [[labour-plan#risks]].\n"
	if bag.Note.Body != want {
		t.Errorf("body = %q, want %q", bag.Note.Body, want)
	}
}

func TestRenameRejectsCollision(t *testing.T) {
	s := testStore(t)
	mustSave(t, s, &vault.Note{Slug: "a", Folder: vault.FolderNotes, Frontmatter: vault.Frontmatter{Description: "a"}, Body: "a\n"})
	mustSave(t, s, &vault.Note{Slug: "b", Folder: vault.FolderNotes, Frontmatter: vault.Frontmatter{Description: "b"}, Body: "b\n"})
	if err := s.Rename("test", "a", "b"); err == nil {
		t.Fatal("Rename allowed a collision")
	}
}

func TestAddToHubCreatesAndRegisters(t *testing.T) {
	s := testStore(t)
	mustSave(t, s, &vault.Note{
		Slug: "birth-plan", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "The plan."}, Body: "Plan.\n",
	})

	if err := s.AddToHub("test", "health", "birth-plan", "the birth plan"); err != nil {
		t.Fatalf("AddToHub: %v", err)
	}

	hub, err := s.Get("health")
	if err != nil {
		t.Fatalf("Get hub: %v", err)
	}
	if hub.Note.Folder != vault.FolderHubs || hub.Note.Type() != vault.TypeHub {
		t.Errorf("hub note wrong shape: folder=%s type=%s", hub.Note.Folder, hub.Note.Type())
	}
	if !strings.Contains(hub.Note.Body, "[[birth-plan]]") {
		t.Errorf("hub body missing entry: %q", hub.Note.Body)
	}

	root, err := s.Get("root")
	if err != nil {
		t.Fatalf("Get root: %v", err)
	}
	if !strings.Contains(root.Note.Body, "[[health]]") {
		t.Errorf("root hub missing new hub: %q", root.Note.Body)
	}
	if strings.Contains(root.Note.Body, "No hubs yet.") {
		t.Error("root hub placeholder not removed")
	}

	if err := s.AddToHub("test", "health", "birth-plan", "dup"); err != nil {
		t.Fatalf("second AddToHub: %v", err)
	}
	hub, _ = s.Get("health")
	if strings.Count(hub.Note.Body, "[[birth-plan]]") != 1 {
		t.Errorf("duplicate hub entry: %q", hub.Note.Body)
	}
}

func TestEditNote(t *testing.T) {
	s := testStore(t)
	mustSave(t, s, &vault.Note{
		Slug: "n", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "old"}, Body: "line one\n",
	})

	desc := "new description"
	appendText := "line two"
	if err := s.EditNote("test", "n", Edit{Description: &desc, Append: &appendText}); err != nil {
		t.Fatalf("EditNote: %v", err)
	}
	view, err := s.Get("n")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if view.Note.Frontmatter.Description != desc {
		t.Errorf("description = %q", view.Note.Frontmatter.Description)
	}
	if view.Note.Body != "line one\nline two\n" {
		t.Errorf("body = %q", view.Note.Body)
	}

	hits, err := s.Search(t.Context(), "new description", 10)
	if err != nil || len(hits) == 0 || hits[0].Slug != "n" {
		t.Errorf("edit not reindexed: hits=%v err=%v", hits, err)
	}
}

func TestStatus(t *testing.T) {
	s := testStore(t)
	if _, err := s.Capture("test", "something", "test"); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	st, err := s.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Hubs != 1 || st.Inbox != 1 || st.Notes != 0 {
		t.Errorf("Status = %+v", st)
	}
}
