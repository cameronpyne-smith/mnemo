package store

import (
	"strings"
	"testing"

	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

type recordedCommit struct {
	actor  string
	action string
	paths  []string
}

type fakeCommitter struct {
	commits []recordedCommit
}

func (f *fakeCommitter) Commit(actor, action string, paths []string) {
	f.commits = append(f.commits, recordedCommit{actor: actor, action: action, paths: paths})
}

func (f *fakeCommitter) last(t *testing.T) recordedCommit {
	t.Helper()
	if len(f.commits) == 0 {
		t.Fatal("no commit recorded")
	}
	return f.commits[len(f.commits)-1]
}

func TestMutationsRecordCommits(t *testing.T) {
	s := testStore(t)
	fc := &fakeCommitter{}
	s.SetCommitter(fc)

	slug, err := s.Capture("mcp", "remember the milk", "claude-code")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	c := fc.last(t)
	if c.actor != "mcp" || !strings.Contains(c.action, "capture "+slug) || !strings.Contains(c.action, "claude-code") {
		t.Errorf("capture commit = %+v", c)
	}
	if len(c.paths) != 1 || c.paths[0] != "inbox/"+slug+".md" {
		t.Errorf("capture paths = %v", c.paths)
	}

	mustSave(t, s, &vault.Note{
		Slug: "shopping", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "Shopping list."},
		Body:        "Milk.\n",
	})
	if c := fc.last(t); c.actor != "test" || c.action != "write shopping" || c.paths[0] != "notes/shopping.md" {
		t.Errorf("save commit = %+v", c)
	}

	body := "Milk and eggs.\n"
	if err := s.EditNote("api", "shopping", Edit{Body: &body}); err != nil {
		t.Fatalf("EditNote: %v", err)
	}
	if c := fc.last(t); c.actor != "api" || c.action != "edit shopping (body)" {
		t.Errorf("edit commit = %+v", c)
	}

	if err := s.AddToHub("filing", "errands", "shopping", "the list"); err != nil {
		t.Fatalf("AddToHub: %v", err)
	}
	c = fc.last(t)
	if c.action != "hub errands += shopping" || len(c.paths) != 2 {
		t.Errorf("hub commit = %+v (want hub + root paths for a new hub)", c)
	}

	if err := s.ArchiveCapture("filing", slug, []string{"shopping"}); err != nil {
		t.Fatalf("ArchiveCapture: %v", err)
	}
	c = fc.last(t)
	if !strings.Contains(c.action, "archive "+slug) || len(c.paths) != 2 {
		t.Errorf("archive commit = %+v", c)
	}

	if err := s.Rename("api", "shopping", "groceries"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	c = fc.last(t)
	if !strings.Contains(c.action, "rename shopping -> groceries") {
		t.Errorf("rename commit = %+v", c)
	}
	found := false
	for _, p := range c.paths {
		if p == "hubs/errands.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("rename paths missing rewritten hub: %v", c.paths)
	}
}
