package gitsync

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	branch       = "main"
	pushInterval = 30 * time.Second
	minBackoff   = 30 * time.Second
	maxBackoff   = 15 * time.Minute
	cmdTimeout   = 2 * time.Minute
)

// Syncer records the daemon's own vault mutations as git commits and pushes
// them to configured remotes. Changes made outside the daemon are not its
// concern: the user commits those manually, and pathspec-limited commits
// ensure they are never swept into a daemon commit for an untouched file.
type Syncer struct {
	dir     string
	log     *slog.Logger
	remotes map[string]string

	mu      sync.Mutex
	commits int
	lastErr string
	state   map[string]*remoteState
	wake    chan struct{}
}

type remoteState struct {
	lastPush time.Time
	lastErr  string
	backoff  time.Duration
	nextTry  time.Time
}

type RemoteStatus struct {
	Name      string
	Lag       int
	LastPush  time.Time
	LastError string
}

type Status struct {
	Commits   int
	LastError string
	Remotes   []RemoteStatus
}

func New(dir string, remotes map[string]string, log *slog.Logger) *Syncer {
	s := &Syncer{
		dir:     dir,
		log:     log,
		remotes: remotes,
		state:   make(map[string]*remoteState, len(remotes)),
		wake:    make(chan struct{}, 1),
	}
	for name := range remotes {
		s.state[name] = &remoteState{}
	}
	return s
}

func (s *Syncer) Init(ctx context.Context) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git binary not found — redundancy requires git on this machine, or set [git] enabled = false in config: %w", err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, ".git")); os.IsNotExist(err) {
		if _, err := s.git(ctx, "init", "--quiet"); err != nil {
			return err
		}
		if _, err := s.git(ctx, "symbolic-ref", "HEAD", "refs/heads/"+branch); err != nil {
			return err
		}
	}
	local := map[string]string{
		"user.name":      "mnemo",
		"user.email":     "mnemo@vault",
		"commit.gpgsign": "false",
		"core.autocrlf":  "false",
	}
	for key, value := range local {
		if _, err := s.git(ctx, "config", key, value); err != nil {
			return err
		}
	}
	ignorePath := filepath.Join(s.dir, ".gitignore")
	if _, err := os.Stat(ignorePath); os.IsNotExist(err) {
		if err := os.WriteFile(ignorePath, []byte(".mnemo-*.tmp\n.mnemo/\n"), 0o644); err != nil {
			return fmt.Errorf("writing .gitignore: %w", err)
		}
	}
	for name, url := range s.remotes {
		if _, err := s.git(ctx, "remote", "get-url", name); err != nil {
			if _, err := s.git(ctx, "remote", "add", name, url); err != nil {
				return err
			}
		} else if _, err := s.git(ctx, "remote", "set-url", name, url); err != nil {
			return err
		}
	}
	if _, err := s.git(ctx, "rev-parse", "--verify", "HEAD"); err != nil {
		if err := s.commitAll(ctx, "mnemo: baseline of existing vault state"); err != nil {
			return err
		}
	}
	// Commit-only repos never trigger git's auto maintenance (only fetch,
	// merge, and friends do), so give gc --auto its one chance per daemon run.
	_, _ = s.git(ctx, "gc", "--auto", "--quiet")
	return nil
}

// Commit records a single mutation. Limited to the given repo-relative paths
// so concurrent external changes are never misattributed to this actor.
// Failures are logged and surfaced via Stats, never returned: git is the
// redundancy layer, and a git hiccup must not fail a vault write.
func (s *Syncer) Commit(actor, action string, paths []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	statusArgs := append([]string{"status", "--porcelain", "--"}, paths...)
	out, err := s.git(ctx, statusArgs...)
	if err != nil {
		s.fail("status before commit", err)
		return
	}
	if strings.TrimSpace(out) == "" {
		return
	}
	if _, err := s.git(ctx, append([]string{"add", "--"}, paths...)...); err != nil {
		s.fail("add", err)
		return
	}
	commitArgs := append([]string{"commit", "--quiet", "-m", actor + ": " + action, "--"}, paths...)
	if _, err := s.git(ctx, commitArgs...); err != nil {
		s.fail("commit", err)
		return
	}
	s.commits++
	s.lastErr = ""
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Syncer) fail(op string, err error) {
	s.lastErr = err.Error()
	if s.log != nil {
		s.log.Error("gitsync "+op+" failed", "error", err)
	}
}

// Run pushes committed history to the remotes until ctx ends. It exists so a
// slow, unplugged, or unreachable remote never blocks a vault write: commits
// wake it, failures retry with backoff, a reconnected remote catches up fully.
func (s *Syncer) Run(ctx context.Context) {
	ticker := time.NewTicker(pushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.shutdown()
			return
		case <-ticker.C:
		case <-s.wake:
		}
		s.tick(ctx)
	}
}

func (s *Syncer) tick(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pushDueLocked(ctx)
}

func (s *Syncer) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	for name := range s.remotes {
		s.state[name].nextTry = time.Time{}
	}
	s.pushDueLocked(ctx)
}

func (s *Syncer) pushDueLocked(ctx context.Context) {
	now := time.Now()
	for name := range s.remotes {
		st := s.state[name]
		if now.Before(st.nextTry) {
			continue
		}
		lag := s.lagLocked(ctx, name)
		if lag == 0 {
			continue
		}
		if _, err := s.git(ctx, "push", "--quiet", name, branch); err != nil {
			st.backoff *= 2
			if st.backoff < minBackoff {
				st.backoff = minBackoff
			}
			if st.backoff > maxBackoff {
				st.backoff = maxBackoff
			}
			st.nextTry = now.Add(st.backoff)
			if st.lastErr != err.Error() && s.log != nil {
				s.log.Warn("push failed", "remote", name, "retry_in", st.backoff, "error", err)
			}
			st.lastErr = err.Error()
			continue
		}
		st.backoff = 0
		st.nextTry = time.Time{}
		st.lastErr = ""
		st.lastPush = now
		if s.log != nil {
			s.log.Info("pushed", "remote", name, "commits", lag)
		}
	}
}

func (s *Syncer) lagLocked(ctx context.Context, name string) int {
	out, err := s.git(ctx, "rev-list", "--count", name+"/"+branch+".."+branch)
	if err != nil {
		out, err = s.git(ctx, "rev-list", "--count", branch)
		if err != nil {
			return 0
		}
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n
}

func (s *Syncer) commitAll(ctx context.Context, msg string) error {
	out, err := s.git(ctx, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return nil
	}
	if _, err := s.git(ctx, "add", "-A"); err != nil {
		return err
	}
	if _, err := s.git(ctx, "commit", "--quiet", "-m", msg); err != nil {
		return err
	}
	s.commits++
	return nil
}

func (s *Syncer) Stats(ctx context.Context) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Commits: s.commits, LastError: s.lastErr}
	names := make([]string, 0, len(s.remotes))
	for name := range s.remotes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rs := s.state[name]
		st.Remotes = append(st.Remotes, RemoteStatus{
			Name:      name,
			Lag:       s.lagLocked(ctx, name),
			LastPush:  rs.lastPush,
			LastError: rs.lastErr,
		})
	}
	return st
}

func (s *Syncer) git(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", s.dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// InitBare creates an append-only bare repository suitable as a push target:
// history can be added but never rewritten or deleted, even by a compromised
// pusher.
func InitBare(path string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git binary not found: %w", err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	steps := [][]string{
		{"init", "--bare", "--quiet", path},
		{"-C", path, "config", "receive.denyNonFastForwards", "true"},
		{"-C", path, "config", "receive.denyDeletes", "true"},
	}
	for _, args := range steps {
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
