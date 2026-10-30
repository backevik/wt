package app

import (
	"fmt"
	"strings"

	"github.com/backevik/wt/internal/github"
	"github.com/backevik/wt/internal/state"
)

// Status is what wt knows about a worktree when deciding whether it can go.
type Status struct {
	Worktree    state.Worktree
	SessionOpen bool
	// Head is the branch's current commit.
	Head string
	// Changes are uncommitted and untracked files. Ignored files don't count.
	Changes []string
	// Unpushed counts commits no remote branch has.
	Unpushed int
	PR       *github.PR
	// MergedIntoMain is true when the branch has commits of its own and they
	// are all in the default branch.
	MergedIntoMain bool
	// Err is set when git couldn't read the status. Such a worktree is never
	// removed by clean.
	Err error
	// PRErr is set when the PR lookup failed, such as for a repo that isn't
	// on GitHub. Merged-into-main still counts then.
	PRErr error
}

// Finished reports whether the branch's work has landed or been abandoned:
// its PR is merged or closed, or it is merged into the default branch.
func (s Status) Finished() bool {
	return s.PR.Done() || s.MergedIntoMain
}

// WorkIsSafe reports whether every commit on the branch exists somewhere
// else. A squash-merged PR whose remote branch was deleted leaves commits no
// remote has, but if the PR's head is the local head, GitHub has them.
func (s Status) WorkIsSafe() bool {
	if s.Unpushed == 0 {
		return true
	}
	return s.PR.Done() && s.PR.HeadOID != "" && s.PR.HeadOID == s.Head
}

// Verdict is the outcome of Decide.
type Verdict struct {
	Remove bool
	// Reason explains the verdict for the user.
	Reason string
}

// Decide applies wt clean's rules. A worktree is removed when its work is
// finished, its session is closed, its commits are safe elsewhere, and it has
// no local changes, unless force allows discarding them.
func Decide(s Status, force bool) Verdict {
	switch {
	case s.Err != nil:
		return Verdict{Reason: "couldn't check: " + s.Err.Error()}
	case !s.Finished() && s.PRErr != nil:
		return Verdict{Reason: "not merged into main, and couldn't check the PR: " + s.PRErr.Error()}
	case !s.Finished():
		return Verdict{Reason: unfinishedReason(s.PR)}
	case s.SessionOpen:
		return Verdict{Reason: "Rex session is open"}
	case !s.WorkIsSafe():
		return Verdict{Reason: fmt.Sprintf("%d unpushed commit(s)", s.Unpushed)}
	case len(s.Changes) > 0 && !force:
		return Verdict{Reason: fmt.Sprintf("%d local change(s), use --force to discard", len(s.Changes))}
	}
	return Verdict{Remove: true, Reason: finishedReason(s)}
}

func unfinishedReason(pr *github.PR) string {
	if pr == nil {
		return "no PR, not merged into main"
	}
	return fmt.Sprintf("PR #%d is %s", pr.Number, strings.ToLower(pr.State))
}

func finishedReason(s Status) string {
	reason := "merged into main"
	if s.PR.Done() {
		reason = fmt.Sprintf("PR #%d %s", s.PR.Number, strings.ToLower(s.PR.State))
	}
	if len(s.Changes) > 0 {
		reason += fmt.Sprintf(", discarding %d local change(s)", len(s.Changes))
	}
	return reason
}
