package store

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid argument")
)

func (s *Store) Hubs() (*vault.Note, []*vault.Note, error) {
	root, err := s.vault.Read(vault.FolderHubs, "root")
	if err != nil {
		return nil, nil, err
	}
	all, err := s.vault.List(vault.FolderHubs)
	if err != nil {
		return nil, nil, err
	}
	hubs := make([]*vault.Note, 0, len(all))
	for _, n := range all {
		if n.Slug != "root" {
			hubs = append(hubs, n)
		}
	}
	return root, hubs, nil
}

func (s *Store) AddToHub(hubSlug, noteSlug, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !vault.ValidSlug(hubSlug) {
		return fmt.Errorf("add to hub: invalid hub slug %q", hubSlug)
	}
	hub, err := s.vault.Read(vault.FolderHubs, hubSlug)
	if err != nil {
		hub = &vault.Note{
			Slug:   hubSlug,
			Folder: vault.FolderHubs,
			Frontmatter: vault.Frontmatter{
				Description: fmt.Sprintf("Hub for %s.", strings.ReplaceAll(hubSlug, "-", " ")),
				Type:        vault.TypeHub,
			},
			Body: fmt.Sprintf("# %s\n", strings.ReplaceAll(hubSlug, "-", " ")),
		}
		if err := s.registerHubInRootLocked(hub); err != nil {
			return err
		}
	}

	for _, existing := range hub.Links() {
		if existing == noteSlug {
			return nil
		}
	}
	hub.Body = appendEntry(hub.Body, noteSlug, description)
	return s.saveLocked(hub)
}

func (s *Store) registerHubInRootLocked(hub *vault.Note) error {
	root, err := s.vault.Read(vault.FolderHubs, "root")
	if err != nil {
		return fmt.Errorf("registering hub %s: %w", hub.Slug, err)
	}
	for _, existing := range root.Links() {
		if existing == hub.Slug {
			return nil
		}
	}
	if strings.Contains(root.Body, "No hubs yet.") {
		root.Body = strings.Replace(root.Body, "No hubs yet.\n", "", 1)
	}
	root.Body = appendEntry(root.Body, hub.Slug, hub.Frontmatter.Description)
	return s.saveLocked(root)
}

func appendEntry(body, slug, description string) string {
	entry := fmt.Sprintf("- [[%s]] — %s\n", slug, strings.TrimSpace(description))
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body + entry
}
