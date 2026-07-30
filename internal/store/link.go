package store

import (
	"fmt"
	"strings"
)

// Link appends "- [[to]] — context" under from's "## Related" section,
// creating the section at the end of the body when absent. Both notes must
// exist. Linking an already-linked pair is a no-op, so a retried dreamer
// cycle cannot duplicate entries.
func (s *Store) Link(actor, from, to, context string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	folder, ok := s.vault.Locate(from)
	if !ok {
		return fmt.Errorf("link %s -> %s: %w", from, to, ErrNotFound)
	}
	if _, ok := s.vault.Locate(to); !ok {
		return fmt.Errorf("link %s -> %s: target: %w", from, to, ErrNotFound)
	}
	n, err := s.vault.Read(folder, from)
	if err != nil {
		return err
	}
	for _, existing := range n.Links() {
		if existing == to {
			return nil
		}
	}
	if !strings.HasPrefix(n.Body, "## Related") && !strings.Contains(n.Body, "\n## Related") {
		if n.Body != "" && !strings.HasSuffix(n.Body, "\n") {
			n.Body += "\n"
		}
		n.Body += "\n## Related\n"
	}
	n.Body = appendEntry(n.Body, to, context)
	if err := s.saveLocked(n); err != nil {
		return err
	}
	s.record(actor, fmt.Sprintf("link %s -> %s", from, to), notePath(folder, from))
	return nil
}
