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

func (s *Store) EditNote(actor, slug string, edit Edit) error {
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
	var changed []string
	if edit.Description != nil {
		n.Frontmatter.Description = *edit.Description
		changed = append(changed, "description")
	}
	if edit.Tags != nil {
		n.Frontmatter.Tags = *edit.Tags
		changed = append(changed, "tags")
	}
	if edit.Body != nil {
		n.Body = *edit.Body
		changed = append(changed, "body")
	}
	if edit.Append != nil {
		text := strings.TrimRight(*edit.Append, "\n")
		if n.Body != "" && !strings.HasSuffix(n.Body, "\n") {
			n.Body += "\n"
		}
		n.Body += text + "\n"
		changed = append(changed, "append")
	}
	if err := s.saveLocked(n); err != nil {
		return err
	}
	s.record(actor, fmt.Sprintf("edit %s (%s)", slug, strings.Join(changed, ", ")), notePath(folder, slug))
	return nil
}
