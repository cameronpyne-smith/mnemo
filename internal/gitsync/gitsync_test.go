package gitsync

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func testSyncer(t *testing.T, remotes map[string]string) (*Syncer, string) {
	t.Helper()
	requireGit(t)
	dir := t.TempDir()
	writeFile(t, dir, "notes/seed.md", "seed\n")
	s := New(dir, remotes, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return s, dir
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitCount(t *testing.T, dir string) int {
	t.Helper()
	n, err := strconv.Atoi(gitOut(t, dir, "rev-list", "--count", "--all"))
	if err != nil {
		t.Fatalf("parsing rev-list output: %v", err)
	}
	return n
}

func TestInitCreatesRepoWithBaseline(t *testing.T) {
	s, dir := testSyncer(t, nil)

	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("no .git after Init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err != nil {
		t.Errorf("no .gitignore after Init: %v", err)
	}
	if got := commitCount(t, dir); got != 1 {
		t.Errorf("commits after Init = %d, want 1 baseline", got)
	}
	if out := gitOut(t, dir, "status", "--porcelain"); out != "" {
		t.Errorf("tree dirty after baseline: %q", out)
	}
	if branch := gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); branch != "main" {
		t.Errorf("branch = %q, want main", branch)
	}
	if s.commits != 1 {
		t.Errorf("commit counter = %d, want 1", s.commits)
	}

	writeFile(t, dir, "notes/external.md", "edited outside the daemon\n")
	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if got := commitCount(t, dir); got != 1 {
		t.Errorf("commits after re-Init = %d, want still 1 (external edits are not mnemo's to commit)", got)
	}
	if out := gitOut(t, dir, "status", "--porcelain"); !strings.Contains(out, "external.md") {
		t.Errorf("external edit should stay dirty, status = %q", out)
	}
}

func TestCommitOnlyTouchesGivenPaths(t *testing.T) {
	s, dir := testSyncer(t, nil)
	writeFile(t, dir, "notes/a.md", "a\n")
	writeFile(t, dir, "notes/b.md", "b\n")

	s.Commit("test", "write a", []string{"notes/a.md"})

	if msg := gitOut(t, dir, "log", "-1", "--format=%s"); msg != "test: write a" {
		t.Errorf("commit message = %q", msg)
	}
	status := gitOut(t, dir, "status", "--porcelain")
	if !strings.Contains(status, "b.md") || strings.Contains(status, "a.md") {
		t.Errorf("status after pathspec commit = %q, want only b.md dirty", status)
	}
}

func TestCommitNoChangesIsNoop(t *testing.T) {
	s, dir := testSyncer(t, nil)
	before := commitCount(t, dir)

	s.Commit("test", "nothing", []string{"notes/seed.md"})

	if got := commitCount(t, dir); got != before {
		t.Errorf("commits = %d, want %d (no-op)", got, before)
	}
	if s.lastErr != "" {
		t.Errorf("lastErr = %q, want empty", s.lastErr)
	}
}

func TestCommitLeavesForeignDirtyFilesAlone(t *testing.T) {
	s, dir := testSyncer(t, nil)
	writeFile(t, dir, "notes/mine.md", "daemon write\n")
	writeFile(t, dir, "notes/yours.md", "external edit\n")

	s.Commit("test", "write mine", []string{"notes/mine.md"})
	s.tick(context.Background())

	status := gitOut(t, dir, "status", "--porcelain")
	if !strings.Contains(status, "yours.md") {
		t.Errorf("external file should stay uncommitted, status = %q", status)
	}
	if strings.Contains(status, "mine.md") {
		t.Errorf("daemon write not committed, status = %q", status)
	}
}

func TestPushRetryAndCatchUp(t *testing.T) {
	requireGit(t)
	remoteDir := filepath.Join(t.TempDir(), "backup.git")
	s, dir := testSyncer(t, map[string]string{"backup": remoteDir})
	ctx := context.Background()

	writeFile(t, dir, "notes/a.md", "a\n")
	s.Commit("test", "write a", []string{"notes/a.md"})
	s.tick(ctx)

	st := s.state["backup"]
	if st.lastErr == "" {
		t.Fatal("push to missing remote should have failed")
	}
	if st.backoff < minBackoff {
		t.Errorf("backoff = %v, want >= %v", st.backoff, minBackoff)
	}

	writeFile(t, dir, "notes/b.md", "b\n")
	s.Commit("test", "write b", []string{"notes/b.md"})

	if err := InitBare(remoteDir); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	s.mu.Lock()
	st.nextTry = time.Time{}
	s.mu.Unlock()
	s.tick(ctx)

	if st.lastErr != "" {
		t.Fatalf("push after remote restored failed: %s", st.lastErr)
	}
	local := gitOut(t, dir, "rev-parse", "main")
	remote := gitOut(t, remoteDir, "rev-parse", "main")
	if local != remote {
		t.Errorf("remote not caught up: local %s remote %s", local, remote)
	}
	if got := s.lagLocked(ctx, "backup"); got != 0 {
		t.Errorf("lag after catch-up = %d, want 0", got)
	}
}

func TestInitBareIsAppendOnly(t *testing.T) {
	requireGit(t)
	dir := filepath.Join(t.TempDir(), "backup.git")
	if err := InitBare(dir); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	for _, key := range []string{"receive.denyNonFastForwards", "receive.denyDeletes"} {
		if got := gitOut(t, dir, "config", key); got != "true" {
			t.Errorf("%s = %q, want true", key, got)
		}
	}
}

func TestStatsReportsLagAndErrors(t *testing.T) {
	remoteDir := filepath.Join(t.TempDir(), "backup.git")
	s, dir := testSyncer(t, map[string]string{"backup": remoteDir})
	ctx := context.Background()

	writeFile(t, dir, "notes/a.md", "a\n")
	s.Commit("test", "write a", []string{"notes/a.md"})

	st := s.Stats(ctx)
	if st.Commits != 2 {
		t.Errorf("Commits = %d, want 2 (baseline + write)", st.Commits)
	}
	if len(st.Remotes) != 1 || st.Remotes[0].Name != "backup" {
		t.Fatalf("Remotes = %+v", st.Remotes)
	}
	if st.Remotes[0].Lag != 2 {
		t.Errorf("Lag = %d, want 2 (never pushed)", st.Remotes[0].Lag)
	}
}
