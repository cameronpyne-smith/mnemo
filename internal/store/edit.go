package store

import (
	"fmt"
	"strings"
)

type Edit struct {
	Description *string
	Tags        *[]string
	Body        *string
	Append      *string
}

func (s *Store) EditNote(slug string, edit Edit) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	folder, ok := s.vault.Locate(slug)
	if !ok {
		return fmt.Errorf("edit %s: %w", slug, ErrNotFound)
	}
	n, err := s.vault.Read(folder, slug)
	if err != nil {
		return err
	}
	if edit.Description != nil {
		n.Frontmatter.Description = *edit.Description
	}
	if edit.Tags != nil {
		n.Frontmatter.Tags = *edit.Tags
	}
	if edit.Body != nil {
		n.Body = *edit.Body
	}
	if edit.Append != nil {
		text := strings.TrimRight(*edit.Append, "\n")
		if n.Body != "" && !strings.HasSuffix(n.Body, "\n") {
			n.Body += "\n"
		}
		n.Body += text + "\n"
	}
	return s.saveLocked(n)
}
