package dashboard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s (%v)", args, raw, err)
	}
	return strings.TrimSpace(string(raw))
}

func syncFixture(t *testing.T) (string, string, func(string) error) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is not installed")
	}
	base := t.TempDir()
	// No inherited identity, credentials, signing, or hooks in fixture repositories.
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(base, "absent-config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	remote := filepath.Join(base, "remote.git")
	work := filepath.Join(base, "用户 checkout")
	testGit(t, base, "init", "--bare", "-b", "main", remote)
	testGit(t, base, "clone", remote, work)
	testGit(t, work, "config", "user.name", "Test")
	testGit(t, work, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("project"), 0600); err != nil {
		t.Fatal(err)
	}
	testGit(t, work, "add", "README.md")
	testGit(t, work, "commit", "-m", "Initial project")
	testGit(t, work, "push", "-u", "origin", "main")
	raw, err := os.ReadFile("testdata/public-usage.json")
	if err != nil {
		t.Fatal(err)
	}
	export := func(path string) error {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		return os.WriteFile(path, raw, 0600)
	}
	return work, remote, export
}

func TestSyncOnlySnapshotAndNoop(t *testing.T) {
	work, remote, export := syncFixture(t)
	private := filepath.Join(work, "private.txt")
	if err := os.WriteFile(private, []byte("do-not-publish"), 0600); err != nil {
		t.Fatal(err)
	}
	testGit(t, work, "add", "private.txt")
	if err := os.WriteFile(private, []byte("new unstaged contents"), 0600); err != nil {
		t.Fatal(err)
	}
	status := testGit(t, work, "status", "--porcelain", "--", "private.txt")
	if err := syncSnapshot(t.Context(), work, export); err != nil {
		t.Fatal(err)
	}
	if testGit(t, work, "status", "--porcelain", "--", "private.txt") != status {
		t.Fatal("unrelated staging changed")
	}
	files := testGit(t, remote, "ls-tree", "-r", "--name-only", "main")
	if strings.Contains(files, "private.txt") || !strings.Contains(files, snapshotPath) {
		t.Fatal("wrong published files", files)
	}
	head := testGit(t, work, "rev-parse", "HEAD")
	if err := syncSnapshot(t.Context(), work, export); err != nil {
		t.Fatal(err)
	}
	if testGit(t, work, "rev-parse", "HEAD") != head {
		t.Fatal("unchanged snapshot created a commit")
	}
}

func TestSyncRetryAfterPushFailure(t *testing.T) {
	work, remote, export := syncFixture(t)
	hook := filepath.Join(remote, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	before := testGit(t, remote, "rev-parse", "main")
	if err := syncSnapshot(t.Context(), work, export); err == nil {
		t.Fatal("push rejection was ignored")
	}
	head := testGit(t, work, "rev-parse", "HEAD")
	if head == before || testGit(t, remote, "rev-parse", "main") != before {
		t.Fatal("rejected push changed remote or lost local commit")
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if err := syncSnapshot(t.Context(), work, export); err != nil {
		t.Fatal(err)
	}
	if testGit(t, remote, "rev-parse", "main") != head || testGit(t, work, "rev-parse", "HEAD") != head {
		t.Fatal("retry did not publish the pending commit")
	}
}

func TestSyncRejectsUnpublishedHistory(t *testing.T) {
	work, remote, export := syncFixture(t)
	before := testGit(t, remote, "rev-parse", "main")
	path := filepath.Join(work, "secret.txt")
	if err := os.WriteFile(path, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	testGit(t, work, "add", "secret.txt")
	testGit(t, work, "commit", "-m", snapshotCommit)
	testGit(t, work, "rm", "secret.txt")
	testGit(t, work, "commit", "-m", snapshotCommit)
	if err := syncSnapshot(t.Context(), work, export); err == nil {
		t.Fatal("non-sync history published")
	}
	if testGit(t, remote, "rev-parse", "main") != before {
		t.Fatal("remote changed")
	}
}

func TestSyncOfflineKeepsExport(t *testing.T) {
	work, remote, export := syncFixture(t)
	testGit(t, work, "remote", "set-url", "origin", remote+"-offline")
	before := testGit(t, work, "rev-parse", "HEAD")
	if err := syncSnapshot(t.Context(), work, export); err == nil {
		t.Fatal("offline fetch succeeded")
	}
	if _, err := readPagesSnapshot(filepath.Join(work, filepath.FromSlash(snapshotPath))); err != nil {
		t.Fatal("offline export lost", err)
	}
	if testGit(t, work, "rev-parse", "HEAD") != before {
		t.Fatal("offline fetch changed branch")
	}
	testGit(t, work, "remote", "set-url", "origin", remote)
	if err := syncSnapshot(t.Context(), work, export); err != nil {
		t.Fatal(err)
	}
}

func TestSyncRejectsRemoteAdvance(t *testing.T) {
	work, remote, export := syncFixture(t)
	other := filepath.Join(t.TempDir(), "other")
	testGit(t, filepath.Dir(other), "clone", remote, other)
	testGit(t, other, "config", "user.name", "Test")
	testGit(t, other, "config", "user.email", "test@example.invalid")
	testGit(t, other, "commit", "--allow-empty", "-m", "Remote update")
	testGit(t, other, "push")
	head := testGit(t, work, "rev-parse", "HEAD")
	if err := syncSnapshot(t.Context(), work, export); err == nil {
		t.Fatal("remote advancement ignored")
	}
	if testGit(t, work, "rev-parse", "HEAD") != head {
		t.Fatal("sync rewrote local branch")
	}
}

func TestSyncRejectsInvalidDataAndLock(t *testing.T) {
	work, remote, _ := syncFixture(t)
	before := testGit(t, remote, "rev-parse", "main")
	bad := func(path string) error {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(`{"access_token":"secret"}`), 0600)
	}
	if err := syncSnapshot(t.Context(), work, bad); err == nil {
		t.Fatal("private JSON accepted")
	}
	if testGit(t, remote, "rev-parse", "main") != before {
		t.Fatal("invalid data pushed")
	}
	lock := filepath.Join(work, ".git", "codex-tally-sync.lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := syncSnapshot(t.Context(), work, func(string) error { called = true; return nil }); err == nil || called {
		t.Fatal("overlapping sync was not blocked")
	}
}

func TestSyncDoesNotPublishRevokedFieldsFromPendingCommit(t *testing.T) {
	work, remote, export := syncFixture(t)
	hook := filepath.Join(remote, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	before := testGit(t, remote, "rev-parse", "main")
	if err := syncSnapshot(t.Context(), work, export); err == nil {
		t.Fatal("push should be rejected")
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	limited := func(path string) error {
		if err := export(path); err != nil {
			return err
		}
		s, err := readPagesSnapshot(path)
		if err != nil {
			return err
		}
		s.Models = nil
		return atomicJSON(path, s)
	}
	if err := syncSnapshot(t.Context(), work, limited); err == nil {
		t.Fatal("revoked model history published")
	}
	if testGit(t, remote, "rev-parse", "main") != before {
		t.Fatal("remote changed")
	}
}

func TestSyncCommandAndLog(t *testing.T) {
	work, _, _ := syncFixture(t)
	root := filepath.Join(t.TempDir(), "Codex 用户 data")
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(work, ".state", "sync.log")
	args := []string{"sync", "-repo", work, "-codex-home", root, "-state", t.TempDir(), "-log", logPath}
	for range 2 {
		if err := Run(args, "0.0.0-dev"); err != nil {
			t.Fatal(err)
		}
	}
	s, err := readPagesSnapshot(filepath.Join(work, filepath.FromSlash(snapshotPath)))
	if err != nil || s.mask() != 7 {
		t.Fatalf("default CLI export: %+v, %v", s, err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(raw), "Public snapshot pushed") || !strings.Contains(string(raw), "nothing to push") {
		t.Fatalf("missing task logs: %s, %v", raw, err)
	}
}

func TestSyncIgnoresForeignIndex(t *testing.T) {
	work, _, export := syncFixture(t)
	foreign := filepath.Join(t.TempDir(), "foreign-index")
	if err := os.WriteFile(foreign, []byte("do-not-touch"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_INDEX_FILE", foreign)
	if err := syncSnapshot(t.Context(), work, export); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(foreign)
	if err != nil || string(raw) != "do-not-touch" {
		t.Fatal("foreign index modified")
	}
}
