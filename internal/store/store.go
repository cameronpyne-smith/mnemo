package store

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cameronpyne-smith/mnemo/internal/embedder"
	"github.com/cameronpyne-smith/mnemo/internal/index"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

type Store struct {
	mu        sync.Mutex
	vault     *vault.Vault
	idx       *index.Index
	now       func() time.Time
	committer Committer
	embed     *embedder.Worker
	query     *embedder.QueryEmbedder
	log       *slog.Logger
}

// Committer records a completed mutation in the redundancy layer. Called
// while the store's mutex is held so commits are attributed exactly; it must
// not call back into the store and must never fail the mutation.
type Committer interface {
	Commit(actor, action string, paths []string)
}

const (
	ActorAPI    = "api"
	ActorMCP    = "mcp"
	ActorFiling = "filing"
)

func (s *Store) SetCommitter(c Committer) { s.committer = c }

// EnableEmbeddings wires semantic search: a background worker keeps the
// index's vectors reconciled with the vault (run its Run in a goroutine),
// mutations wake it, and searches embed their query through the same client.
// Call before serving; a nil log means silent. Without this call, search is
// FTS-only and Similar reports ErrUnavailable.
func (s *Store) EnableEmbeddings(cache *embedder.Cache, client embedder.EmbedClient, queryInstruction string, log *slog.Logger) *embedder.Worker {
	s.embed = embedder.NewWorker(s.vault, s.idx, cache, client, log)
	s.query = embedder.NewQueryEmbedder(client, queryInstruction)
	s.log = log
	return s.embed
}

func (s *Store) wakeEmbed() {
	if s.embed != nil {
		s.embed.Wake()
	}
}

func (s *Store) record(actor, action string, paths ...string) {
	if s.committer != nil {
		s.committer.Commit(actor, action, paths)
	}
}

func notePath(folder, slug string) string { return folder + "/" + slug + ".md" }

type NoteView struct {
	Note      *vault.Note
	Links     []string
	Backlinks []string
}

type Status struct {
	Notes    int
	Hubs     int
	Inbox    int
	Archived int
	Embed    EmbedStatus
}

type EmbedStatus struct {
	Enabled   bool
	Embedded  int
	Backlog   int
	LastError string
}

func Open(path string) (*Store, error) {
	v, err := vault.Open(path)
	if err != nil {
		return nil, err
	}
	idx, err := index.Build(v)
	if err != nil {
		return nil, err
	}
	return &Store{vault: v, idx: idx, now: time.Now}, nil
}

// queryEmbedTimeout bounds how long a search waits on the model before
// degrading to FTS-only. Warm embedding is milliseconds; this only bites when
// ollama is loading or down, where a stalled search is worse than a text one.
const queryEmbedTimeout = 5 * time.Second

// Search is hybrid when embeddings are enabled and any vectors exist: the
// query is embedded (instruction-wrapped) and FTS and vector rankings are
// fused. Any failure on the vector side degrades to FTS-only rather than
// failing the search.
func (s *Store) Search(ctx context.Context, q string, limit int) ([]index.Hit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, fmt.Errorf("search: empty query: %w", ErrInvalid)
	}
	if s.query == nil || s.idx.VectorCount() == 0 {
		return s.idx.Search(q, limit)
	}
	embedCtx, cancel := context.WithTimeout(ctx, queryEmbedTimeout)
	defer cancel()
	queryVec, err := s.query.EmbedQuery(embedCtx, q)
	if err != nil {
		if s.log != nil {
			s.log.Warn("query embedding failed; searching text-only", "error", err)
		}
		return s.idx.Search(q, limit)
	}
	return s.idx.SearchHybrid(q, queryVec, limit)
}

// Similar lists the notes semantically nearest to slug by embedding cosine.
func (s *Store) Similar(slug string, limit int) ([]index.Hit, error) {
	if _, ok := s.vault.Locate(slug); !ok {
		return nil, fmt.Errorf("similar %s: %w", slug, ErrNotFound)
	}
	if s.embed == nil {
		return nil, fmt.Errorf("similar %s: embeddings disabled: %w", slug, ErrUnavailable)
	}
	hits, ok := s.idx.Similar(slug, limit)
	if !ok {
		return nil, fmt.Errorf("similar %s: not embedded yet: %w", slug, ErrUnavailable)
	}
	return hits, nil
}

func (s *Store) Get(slug string) (*NoteView, error) {
	folder, ok := s.vault.Locate(slug)
	if !ok {
		return nil, fmt.Errorf("note %s: %w", slug, ErrNotFound)
	}
	n, err := s.vault.Read(folder, slug)
	if err != nil {
		return nil, err
	}
	return &NoteView{Note: n, Links: n.Links(), Backlinks: s.idx.Backlinks(slug)}, nil
}

func (s *Store) Save(actor string, n *vault.Note) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveLocked(n); err != nil {
		return err
	}
	s.record(actor, "write "+n.Slug, notePath(n.Folder, n.Slug))
	return nil
}

func (s *Store) saveLocked(n *vault.Note) error {
	today := s.now().Format("2006-01-02")
	if n.Frontmatter.Created == "" {
		n.Frontmatter.Created = today
	}
	n.Frontmatter.Updated = today
	if err := s.vault.Write(n); err != nil {
		return err
	}
	if n.Folder == vault.FolderNotes || n.Folder == vault.FolderHubs {
		if err := s.idx.IndexNote(n); err != nil {
			return err
		}
		s.wakeEmbed()
	}
	return nil
}

func (s *Store) Capture(actor, content, source string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("capture: empty content: %w", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := vault.NewCapture(content, source, s.now())
	if err := s.vault.Write(n); err != nil {
		return "", err
	}
	action := "capture " + n.Slug
	if source != "" {
		action += " (source: " + source + ")"
	}
	s.record(actor, action, notePath(vault.FolderInbox, n.Slug))
	return n.Slug, nil
}

func (s *Store) ListNotes() ([]*vault.Note, error) {
	return s.vault.List(vault.FolderNotes)
}

func (s *Store) ListInbox() ([]*vault.Note, error) {
	notes, err := s.vault.List(vault.FolderInbox)
	if err != nil {
		return nil, err
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].Slug < notes[j].Slug })
	return notes, nil
}

func (s *Store) ArchiveCapture(actor, slug string, filedInto []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, err := s.vault.Read(vault.FolderInbox, slug)
	if err != nil {
		return err
	}
	if n.Frontmatter.Extra == nil {
		n.Frontmatter.Extra = make(map[string]any)
	}
	n.Frontmatter.Extra["filed_into"] = filedInto
	n.Frontmatter.Description = "Processed capture, retained for audit."
	n.Folder = vault.FolderArchive
	if err := s.saveLocked(n); err != nil {
		return err
	}
	if err := s.vault.Delete(vault.FolderInbox, slug); err != nil {
		return err
	}
	s.record(actor, fmt.Sprintf("archive %s (filed into %s)", slug, strings.Join(filedInto, ", ")),
		notePath(vault.FolderArchive, slug), notePath(vault.FolderInbox, slug))
	return nil
}

func (s *Store) Rename(actor, oldSlug, newSlug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !vault.ValidSlug(newSlug) {
		return fmt.Errorf("rename: invalid slug %q", newSlug)
	}
	folder, ok := s.vault.Locate(oldSlug)
	if !ok {
		return fmt.Errorf("rename %s: %w", oldSlug, ErrNotFound)
	}
	if _, exists := s.vault.Locate(newSlug); exists {
		return fmt.Errorf("rename: %s already exists", newSlug)
	}

	paths := []string{notePath(folder, oldSlug), notePath(folder, newSlug)}
	rewritten := 0
	for _, src := range s.idx.Backlinks(oldSlug) {
		srcFolder, ok := s.vault.Locate(src)
		if !ok {
			continue
		}
		srcNote, err := s.vault.Read(srcFolder, src)
		if err != nil {
			return fmt.Errorf("rename: rewriting links in %s: %w", src, err)
		}
		srcNote.Body = vault.RewriteLinks(srcNote.Body, oldSlug, newSlug)
		if err := s.saveLocked(srcNote); err != nil {
			return fmt.Errorf("rename: rewriting links in %s: %w", src, err)
		}
		paths = append(paths, notePath(srcFolder, src))
		rewritten++
	}

	n, err := s.vault.Read(folder, oldSlug)
	if err != nil {
		return err
	}
	n.Slug = newSlug
	if err := s.saveLocked(n); err != nil {
		return err
	}
	if err := s.vault.Delete(folder, oldSlug); err != nil {
		return err
	}
	if folder == vault.FolderNotes || folder == vault.FolderHubs {
		if err := s.idx.RemoveNote(oldSlug); err != nil {
			return err
		}
		s.wakeEmbed()
	}
	s.record(actor, fmt.Sprintf("rename %s -> %s (%d links rewritten)", oldSlug, newSlug, rewritten), paths...)
	return nil
}

func (s *Store) Status() (Status, error) {
	var st Status
	counts := map[string]*int{
		vault.FolderNotes:   &st.Notes,
		vault.FolderHubs:    &st.Hubs,
		vault.FolderInbox:   &st.Inbox,
		vault.FolderArchive: &st.Archived,
	}
	for folder, target := range counts {
		notes, err := s.vault.List(folder)
		if err != nil {
			return st, err
		}
		*target = len(notes)
	}
	if s.embed != nil {
		st.Embed = EmbedStatus{
			Enabled:   true,
			Embedded:  s.idx.VectorCount(),
			Backlog:   s.embed.Backlog(),
			LastError: s.embed.LastError(),
		}
	}
	return st, nil
}
