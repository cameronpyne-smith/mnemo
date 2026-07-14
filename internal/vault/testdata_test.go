package vault

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenFixtureVault(t *testing.T) {
	v, err := Open(filepath.Join("testdata", "vault"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	notes, err := v.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(notes) != 3 {
		t.Fatalf("List returned %d notes, want 3", len(notes))
	}

	foreign, err := v.Read(FolderNotes, "obsidian-authored")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if foreign.Frontmatter.Extra["cssclasses"] != "wide" {
		t.Errorf("foreign frontmatter not preserved: %+v", foreign.Frontmatter.Extra)
	}
	wantLinks := []string{"birth-plan", "hospital-bag"}
	if got := foreign.Links(); !reflect.DeepEqual(got, wantLinks) {
		t.Errorf("links = %v, want %v", got, wantLinks)
	}

	bare, err := v.Read(FolderNotes, "bare-note")
	if err != nil {
		t.Fatalf("Read bare note: %v", err)
	}
	if bare.Frontmatter.Description != "" || bare.Body == "" {
		t.Errorf("bare note parsed wrong: %+v", bare)
	}
}
