package store

import (
	"fmt"
	"strings"

	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

type DeleteResult struct {
	Folder          string
	RemovedFromHubs []string
	DanglingLinks   []string
}

// DeleteNote permanently removes a note. Entry lines referencing it are
// stripped from hubs; any other inbound wikilinks are left in place and
// reported as dangling. The git history is the undo.
func (s *Store) DeleteNote(actor, slug string) (*DeleteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if slug == "root" {
		return nil, fmt.Errorf("delete: the root hub cannot be deleted: %w", ErrInvalid)
	}
	folder, ok := s.vault.Locate(slug)
	if !ok {
		return nil, fmt.Errorf("delete %s: %w", slug, ErrNotFound)
	}

	result := &DeleteResult{Folder: folder}
	paths := []string{notePath(folder, slug)}
	for _, src := range s.idx.Backlinks(slug) {
		srcFolder, ok := s.vault.Locate(src)
		if !ok {
			continue
		}
		srcNote, err := s.vault.Read(srcFolder, src)
		if err != nil {
			return nil, fmt.Errorf("delete %s: reading linking note %s: %w", slug, src, err)
		}
		if srcFolder == vault.FolderHubs {
			if stripped, removed := stripEntryLines(srcNote.Body, slug); removed > 0 {
				srcNote.Body = stripped
				if err := s.saveLocked(srcNote); err != nil {
					return nil, fmt.Errorf("delete %s: updating hub %s: %w", slug, src, err)
				}
				paths = append(paths, notePath(srcFolder, src))
				result.RemovedFromHubs = append(result.RemovedFromHubs, src)
			}
		}
		for _, link := range vault.ExtractWikilinks(srcNote.Body) {
			if link == slug {
				result.DanglingLinks = append(result.DanglingLinks, src)
				break
			}
		}
	}

	if err := s.vault.Delete(folder, slug); err != nil {
		return nil, err
	}
	if folder == vault.FolderNotes || folder == vault.FolderHubs {
		if err := s.idx.RemoveNote(slug); err != nil {
			return nil, err
		}
		s.wakeEmbed()
	}
	s.record(actor, fmt.Sprintf("delete %s (removed from %d hub(s))", slug, len(result.RemovedFromHubs)), paths...)
	return result, nil
}

// stripEntryLines drops list lines that lead with a wikilink to slug — the
// `- [[slug]] — description` hub entry shape, tolerating `*` bullets and
// alias/anchor suffixes. Prose mentions elsewhere in the line survive.
func stripEntryLines(body, slug string) (string, int) {
	lines := strings.Split(body, "\n")
	kept := make([]string, 0, len(lines))
	removed := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 2 && (trimmed[0] == '-' || trimmed[0] == '*') && trimmed[1] == ' ' {
			rest := strings.TrimSpace(trimmed[2:])
			if after, ok := strings.CutPrefix(rest, "[["+slug); ok {
				if strings.HasPrefix(after, "]]") || strings.HasPrefix(after, "|") || strings.HasPrefix(after, "#") {
					removed++
					continue
				}
			}
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n"), removed
}
