package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var NoteFolders = []string{FolderNotes, FolderHubs, FolderInbox, FolderArchive}

var allFolders = []string{FolderNotes, FolderHubs, FolderInbox, FolderAttachments, FolderArchive}

type Vault struct {
	Root string
}

func Open(root string) (*Vault, error) {
	for _, folder := range allFolders {
		info, err := os.Stat(filepath.Join(root, folder))
		if err != nil {
			return nil, fmt.Errorf("opening vault at %s: %w", root, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("opening vault at %s: %s is not a directory", root, folder)
		}
	}
	return &Vault{Root: root}, nil
}

func Init(root string) (*Vault, error) {
	for _, folder := range allFolders {
		if err := os.MkdirAll(filepath.Join(root, folder), 0o755); err != nil {
			return nil, fmt.Errorf("initialising vault at %s: %w", root, err)
		}
	}
	v := &Vault{Root: root}

	rootHubPath := v.NotePath(FolderHubs, "root")
	if _, err := os.Stat(rootHubPath); err == nil {
		return v, nil
	}

	today := time.Now().Format("2006-01-02")
	rootHub := &Note{
		Slug:   "root",
		Folder: FolderHubs,
		Frontmatter: Frontmatter{
			Description: "Entry point to the vault. Links to every hub; start here to find anything.",
			Type:        TypeHub,
			Created:     today,
			Updated:     today,
		},
		Body: "# Root\n\nNo hubs yet.\n",
	}
	if err := v.Write(rootHub); err != nil {
		return nil, fmt.Errorf("initialising vault at %s: %w", root, err)
	}
	return v, nil
}

func (v *Vault) NotePath(folder, slug string) string {
	return filepath.Join(v.Root, folder, slug+".md")
}

func (v *Vault) Read(folder, slug string) (*Note, error) {
	content, err := os.ReadFile(v.NotePath(folder, slug))
	if err != nil {
		return nil, fmt.Errorf("reading note %s/%s: %w", folder, slug, err)
	}
	fm, body, err := ParseNote(content)
	if err != nil {
		return nil, fmt.Errorf("reading note %s/%s: %w", folder, slug, err)
	}
	return &Note{Slug: slug, Folder: folder, Frontmatter: fm, Body: body}, nil
}

func (v *Vault) Write(n *Note) error {
	if !ValidSlug(n.Slug) {
		return fmt.Errorf("writing note: invalid slug %q", n.Slug)
	}
	content, err := EncodeNote(n.Frontmatter, n.Body)
	if err != nil {
		return fmt.Errorf("writing note %s/%s: %w", n.Folder, n.Slug, err)
	}
	if err := atomicWrite(v.NotePath(n.Folder, n.Slug), content); err != nil {
		return fmt.Errorf("writing note %s/%s: %w", n.Folder, n.Slug, err)
	}
	return nil
}

func (v *Vault) List(folders ...string) ([]*Note, error) {
	if len(folders) == 0 {
		folders = NoteFolders
	}
	var notes []*Note
	for _, folder := range folders {
		entries, err := os.ReadDir(filepath.Join(v.Root, folder))
		if err != nil {
			return nil, fmt.Errorf("listing %s: %w", folder, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			slug := strings.TrimSuffix(entry.Name(), ".md")
			note, err := v.Read(folder, slug)
			if err != nil {
				return nil, err
			}
			notes = append(notes, note)
		}
	}
	return notes, nil
}

func (v *Vault) Locate(slug string) (string, bool) {
	for _, folder := range NoteFolders {
		if _, err := os.Stat(v.NotePath(folder, slug)); err == nil {
			return folder, true
		}
	}
	return "", false
}

func atomicWrite(path string, content []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mnemo-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
