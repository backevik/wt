// Package git runs the git commands wt needs.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Git runs git. Bin defaults to "git".
type Git struct {
	Bin string
}

func (g Git) run(ctx context.Context, dir string, args ...string) (string, error) {
	bin := g.Bin
	if bin == "" {
		bin = "git"
	}
	cmd := exec.CommandContext(ctx, bin, append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", &Error{Args: args, Msg: msg, Err: err}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Error is a failed git command.
type Error struct {
	Args []string
	Msg  string
	Err  error
}

func (e *Error) Error() string { return "git " + strings.Join(e.Args, " ") + ": " + e.Msg }
func (e *Error) Unwrap() error { return e.Err }

func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// MainRepo returns the main working tree of the repository dir belongs to,
// so a linked worktree resolves to the repo it was created from.
func (g Git) MainRepo(ctx context.Context, dir string) (string, error) {
	common, err := g.run(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if filepath.Base(common) != ".git" {
		return "", fmt.Errorf("%s is a bare repository, which wt doesn't support", common)
	}
	return filepath.Dir(common), nil
}

// Fetch updates the remote-tracking branches from origin.
func (g Git) Fetch(ctx context.Context, repo string) error {
	_, err := g.run(ctx, repo, "fetch", "--prune", "--quiet", "origin")
	return err
}

// DefaultBranch returns origin's default branch, such as origin/main.
func (g Git) DefaultBranch(ctx context.Context, repo string) string {
	ref, err := g.run(ctx, repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil || ref == "" {
		return "origin/main"
	}
	return ref
}

// BranchExists reports whether a local branch exists.
func (g Git) BranchExists(ctx context.Context, repo, branch string) (bool, error) {
	_, err := g.run(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err == nil {
		return true, nil
	}
	if exitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

// RevParse resolves a ref to a commit hash.
func (g Git) RevParse(ctx context.Context, dir, ref string) (string, error) {
	return g.run(ctx, dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
}

// AddWorktree creates a worktree at path. With base set, it creates branch
// from base without tracking it, so pushing sets the real upstream later.
// Without base, it checks out the existing branch.
func (g Git) AddWorktree(ctx context.Context, repo, path, branch, base string) error {
	args := []string{"worktree", "add", "--quiet"}
	if base != "" {
		args = append(args, "--no-track", "-b", branch, path, base)
	} else {
		args = append(args, path, branch)
	}
	_, err := g.run(ctx, repo, args...)
	return err
}

// RemoveWorktree removes a worktree. Force discards local changes.
func (g Git) RemoveWorktree(ctx context.Context, repo, path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	_, err := g.run(ctx, repo, args...)
	return err
}

// DeleteBranch deletes a local branch, whether or not git considers it merged.
func (g Git) DeleteBranch(ctx context.Context, repo, branch string) error {
	_, err := g.run(ctx, repo, "branch", "--quiet", "-D", branch)
	return err
}

// CurrentBranch returns the branch checked out at path, or "" when detached.
func (g Git) CurrentBranch(ctx context.Context, path string) (string, error) {
	return g.run(ctx, path, "branch", "--show-current")
}

// Changes lists uncommitted and untracked files at path. Ignored files don't count.
func (g Git) Changes(ctx context.Context, path string) ([]string, error) {
	out, err := g.run(ctx, path, "status", "--porcelain")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// Unpushed counts the commits on branch that no remote branch has.
func (g Git) Unpushed(ctx context.Context, repo, branch string) (int, error) {
	out, err := g.run(ctx, repo, "rev-list", "--count", "refs/heads/"+branch, "--not", "--remotes")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(out)
}

// MergeBase returns the best common ancestor of two refs.
func (g Git) MergeBase(ctx context.Context, repo, a, b string) (string, error) {
	return g.run(ctx, repo, "merge-base", a, b)
}

// PruneWorktrees forgets worktrees whose folders no longer exist.
func (g Git) PruneWorktrees(ctx context.Context, repo string) error {
	_, err := g.run(ctx, repo, "worktree", "prune")
	return err
}

// IsAncestor reports whether commit is reachable from ref.
func (g Git) IsAncestor(ctx context.Context, repo, commit, ref string) (bool, error) {
	_, err := g.run(ctx, repo, "merge-base", "--is-ancestor", commit, ref)
	if err == nil {
		return true, nil
	}
	if exitCode(err) == 1 {
		return false, nil
	}
	return false, err
}
