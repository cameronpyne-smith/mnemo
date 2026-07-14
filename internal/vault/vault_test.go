package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesSkeletonAndRootHub(t *testing.T) {
	root := t.TempDir()
	v, err := Init(root)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, folder := range allFolders {
		info, err := os.Stat(filepath.Join(root, folder))
		if err != nil || !info.IsDir() {
			t.Errorf("folder %s missing after Init", folder)
		}
	}
	hub, err := v.Read(FolderHubs, "root")
	if err != nil {
		t.Fatalf("reading root hub: %v", err)
	}
	if hub.Type() != TypeHub {
		t.Errorf("root hub type = %q, want %q", hub.Type(), TypeHub)
	}
	if hub.Frontmatter.Description == "" {
		t.Error("root hub has no description")
	}
}

func TestInitIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	v, err := Init(root)
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	hub, err := v.Read(FolderHubs, "root")
	if err != nil {
		t.Fatalf("reading root hub: %v", err)
	}
	hub.Body = "# Root\n\n[[custom-hub]] — a custom hub.\n"
	if err := v.Write(hub); err != nil {
		t.Fatalf("writing root hub: %v", err)
	}
	if _, err := Init(root); err != nil {
		t.Fatalf("third Init: %v", err)
	}
	reread, err := v.Read(FolderHubs, "root")
	if err != nil {
		t.Fatalf("rereading root hub: %v", err)
	}
	if reread.Body != hub.Body {
		t.Error("Init overwrote an existing root hub")
	}
}

func TestOpenFailsOnMissingSkeleton(t *testing.T) {
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("Open succeeded on an empty directory")
	}
}

func TestWriteReadListLocate(t *testing.T) {
	v, err := Init(t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	note := &Note{
		Slug:   "birth-plan",
		Folder: FolderNotes,
		Frontmatter: Frontmatter{
			Description: "Plan for the birth.",
			Tags:        []string{"health"},
			Created:     "2026-07-14",
		},
		Body: "Linked to [[hospital-bag]].\n",
	}
	if err := v.Write(note); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := v.Read(FolderNotes, "birth-plan")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Frontmatter.Description != note.Frontmatter.Description {
		t.Errorf("description = %q, want %q", got.Frontmatter.Description, note.Frontmatter.Description)
	}
	if links := got.Links(); len(links) != 1 || links[0] != "hospital-bag" {
		t.Errorf("links = %v, want [hospital-bag]", got.Links())
	}

	notes, err := v.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(notes) != 2 {
		t.Errorf("List returned %d notes, want 2 (root hub + birth-plan)", len(notes))
	}

	folder, ok := v.Locate("birth-plan")
	if !ok || folder != FolderNotes {
		t.Errorf("Locate(birth-plan) = %q, %v; want %q, true", folder, ok, FolderNotes)
	}
	if _, ok := v.Locate("missing"); ok {
		t.Error("Locate(missing) reported a hit")
	}
}

func TestWriteRejectsInvalidSlug(t *testing.T) {
	v, err := Init(t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	bad := &Note{Slug: "Bad Slug!", Folder: FolderNotes, Frontmatter: Frontmatter{Description: "d"}}
	if err := v.Write(bad); err == nil {
		t.Fatal("Write accepted an invalid slug")
	}
}

func TestWriteIsAtomicReplace(t *testing.T) {
	v, err := Init(t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	note := &Note{Slug: "n", Folder: FolderNotes, Frontmatter: Frontmatter{Description: "v1"}}
	if err := v.Write(note); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	note.Frontmatter.Description = "v2"
	if err := v.Write(note); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	got, err := v.Read(FolderNotes, "n")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Frontmatter.Description != "v2" {
		t.Errorf("description = %q, want v2", got.Frontmatter.Description)
	}
	entries, err := os.ReadDir(filepath.Join(v.Root, FolderNotes))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("notes dir has %d entries, want 1 (no leftover temp files)", len(entries))
	}
}
