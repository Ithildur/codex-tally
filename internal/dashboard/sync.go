package dashboard

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const snapshotPath = "site/usage.json"
const snapshotCommit = "Update public usage\n\nCodex-Tally-Snapshot: 1"

type gitRepo struct {
	path string
	ctx  context.Context
}

func (g gitRepo) run(args ...string) (string, error) {
	cmd := exec.CommandContext(g.ctx, "git", append([]string{"-C", g.path}, args...)...)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_PREFIX", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES":
			continue // -repo owns the Git context, even when invoked from a hook.
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never")
	cmd.WaitDelay = time.Second
	raw, err := cmd.CombinedOutput()
	if err != nil {
		// Git errors can include credentials embedded in a remote URL. Keep logs
		// safe for scheduled jobs; reproduce the command interactively to debug.
		return "", fmt.Errorf("git %s failed: %w (run it in the repository to inspect authentication or conflicts)", args[0], err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// The exporter writes the one owned path. Other worktree files and staged
// changes are not part of a sync commit, and arbitrary pending commits cannot
// escape through a later push.
func syncSnapshot(ctx context.Context, path string, export func(string) error) error {
	g := gitRepo{path: path, ctx: ctx}
	top, err := g.run("rev-parse", "--show-toplevel")
	if err != nil {
		return errors.New("sync requires Git and a cloned working tree; check -repo")
	}
	g.path = top
	common, err := g.run("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	lock := filepath.Join(common, "codex-tally-sync.lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("sync already running; if a previous process was killed, remove %s after checking", lock)
		}
		return err
	}
	defer os.Remove(lock)
	branch, err := g.run("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return errors.New("sync requires a checked-out branch, not detached HEAD")
	}
	head, err := g.run("rev-parse", "HEAD^{commit}")
	if err != nil {
		return errors.New("commit and push the project before syncing snapshots")
	}
	for _, name := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply"} {
		path, err := g.run("rev-parse", "--path-format=absolute", "--git-path", name)
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err == nil {
			return errors.New("finish the current Git merge/rebase operation before syncing")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	unmerged, err := g.run("ls-files", "-u")
	if err != nil {
		return err
	}
	if unmerged != "" {
		return errors.New("resolve Git conflicts before syncing")
	}
	upstream, err := g.run("for-each-ref", "--format=%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads/"+branch)
	if err != nil {
		return err
	}
	remote, ref, ok := strings.Cut(upstream, "\x00")
	if !ok || remote == "" || remote == "." || strings.HasPrefix(remote, "-") || !strings.HasPrefix(ref, "refs/heads/") {
		return errors.New("configure this branch to track your fork's remote branch before syncing")
	}
	// Fetch and push must describe the same repository, with one destination.
	fetchURL, err := g.run("remote", "get-url", "--all", remote)
	if err != nil {
		return err
	}
	pushURL, err := g.run("remote", "get-url", "--push", "--all", remote)
	if err != nil {
		return err
	}
	if fetchURL != pushURL || strings.Contains(fetchURL, "\n") {
		return errors.New("sync requires one matching fetch/push URL on the tracked remote")
	}
	output := filepath.Join(top, filepath.FromSlash(snapshotPath))
	for _, path := range []string{filepath.Dir(output), output} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("sync snapshot path must not contain symlinks")
		}
	}
	if err := export(output); err != nil {
		return err
	}
	snapshot, err := readPagesSnapshot(output)
	if err != nil {
		return fmt.Errorf("invalid exported snapshot: %w", err)
	}
	log.Print("Public snapshot ready: site/usage.json")
	// Export happens first so an offline machine still keeps the new snapshot.
	if _, err := g.run("fetch", "--no-tags", remote, ref); err != nil {
		return err
	}
	base, err := g.run("rev-parse", "FETCH_HEAD^{commit}")
	if err != nil {
		return err
	}
	if _, err := g.run("merge-base", "--is-ancestor", base, head); err != nil {
		return errors.New("remote has commits not in this branch; update it manually (git pull --ff-only) and retry; snapshot kept")
	}
	if err := g.checkPending(base, head, snapshot.mask()); err != nil {
		return err
	}
	if err := g.sameHead(branch, head); err != nil {
		return err
	}
	if _, err := g.run("add", "--", snapshotPath); err != nil {
		return err
	}
	changed, err := g.run("diff", "--cached", "--name-only", "HEAD", "--", snapshotPath)
	if err != nil {
		return err
	}
	if changed != "" {
		if _, err := g.run("commit", "--only", "-m", snapshotCommit, "--", snapshotPath); err != nil {
			return err
		}
		head, err = g.run("rev-parse", "HEAD^{commit}")
		if err != nil {
			return err
		}
	}
	// Recheck after commit hooks. Inspect each pending commit, not just the final
	// tree: otherwise adding then removing a secret could still publish it.
	if err := g.checkPending(base, head, snapshot.mask()); err != nil {
		return err
	}
	if err := g.sameHead(branch, head); err != nil {
		return err
	}
	if head == base {
		log.Print("Public snapshot unchanged; nothing to push")
		return nil
	}
	// Push the checked object ID, never a moving HEAD or additional branches/tags.
	if _, err := g.run("push", "--no-follow-tags", remote, head+":"+ref); err != nil {
		return fmt.Errorf("snapshot committed locally; next sync will retry: %w", err)
	}
	log.Print("Public snapshot pushed")
	return nil
}

func (g gitRepo) sameHead(branch, head string) error {
	currentBranch, err := g.run("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return err
	}
	current, err := g.run("rev-parse", "HEAD^{commit}")
	if err != nil {
		return err
	}
	if currentBranch != branch || current != head {
		return errors.New("Git branch changed during sync; retry without concurrent Git operations")
	}
	return nil
}

func (g gitRepo) checkPending(base, head string, allowed int) error {
	commits, err := g.run("rev-list", base+".."+head)
	if err != nil {
		return err
	}
	for commit := range strings.FieldsSeq(commits) {
		parents, err := g.run("rev-list", "--parents", "-n", "1", commit)
		if err != nil {
			return err
		}
		message, err := g.run("show", "-s", "--format=%B", commit)
		if err != nil {
			return err
		}
		files, err := g.run("diff-tree", "--no-commit-id", "--name-only", "-r", commit)
		if err != nil {
			return err
		}
		if len(strings.Fields(parents)) != 2 || message != snapshotCommit || files != snapshotPath {
			return errors.New("branch contains unpublished non-sync commits; review and push them manually before sync")
		}
		entry, err := g.run("ls-tree", commit, "--", snapshotPath)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(entry, "100644 blob ") {
			return errors.New("pending snapshot must be a regular non-executable file")
		}
		raw, err := g.run("show", commit+":"+snapshotPath)
		if err != nil {
			return err
		}
		snapshot, err := decodePagesSnapshot([]byte(raw))
		if err != nil {
			return fmt.Errorf("pending snapshot invalid: %w", err)
		}
		if snapshot.mask()&allowed != snapshot.mask() {
			return errors.New("an unpublished snapshot includes fields no longer selected; review local history before syncing")
		}
	}
	return nil
}
