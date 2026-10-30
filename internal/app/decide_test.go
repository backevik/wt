package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/backevik/wt/internal/github"
)

func TestDecide(t *testing.T) {
	merged := &github.PR{Number: 1, State: github.StateMerged, HeadOID: "abc"}
	open := &github.PR{Number: 2, State: github.StateOpen}

	tests := []struct {
		name   string
		status Status
		force  bool
		remove bool
		reason string
	}{
		{
			name:   "merged PR",
			status: Status{PR: merged, Head: "abc"},
			remove: true,
			reason: "PR #1 merged",
		},
		{
			name:   "closed PR",
			status: Status{PR: &github.PR{Number: 3, State: github.StateClosed}},
			remove: true,
			reason: "PR #3 closed",
		},
		{
			name:   "merged into main without PR",
			status: Status{MergedIntoMain: true},
			remove: true,
			reason: "merged into main",
		},
		{
			name:   "open PR",
			status: Status{PR: open},
			reason: "PR #2 is open",
		},
		{
			name:   "no PR and not merged",
			status: Status{},
			reason: "no PR, not merged into main",
		},
		{
			name:   "session open",
			status: Status{PR: merged, Head: "abc", SessionOpen: true},
			reason: "Rex session is open",
		},
		{
			name:   "unpushed commits",
			status: Status{MergedIntoMain: true, Unpushed: 2},
			reason: "2 unpushed commit(s)",
		},
		{
			name:   "squash-merged PR whose remote branch is gone",
			status: Status{PR: merged, Head: "abc", Unpushed: 3},
			remove: true,
		},
		{
			name:   "commits after the PR was merged",
			status: Status{PR: merged, Head: "def", Unpushed: 1},
			reason: "1 unpushed commit(s)",
		},
		{
			name:   "local changes",
			status: Status{PR: merged, Head: "abc", Changes: []string{" M a.go"}},
			reason: "use --force",
		},
		{
			name:   "local changes with force",
			status: Status{PR: merged, Head: "abc", Changes: []string{" M a.go"}},
			force:  true,
			remove: true,
			reason: "discarding 1 local change(s)",
		},
		{
			name:   "status error",
			status: Status{PR: merged, Err: errors.New("boom")},
			force:  true,
			reason: "couldn't check: boom",
		},
		{
			name:   "PR lookup failed but merged into main",
			status: Status{MergedIntoMain: true, PRErr: errors.New("not a GitHub repo")},
			remove: true,
			reason: "merged into main",
		},
		{
			name:   "PR lookup failed and not merged",
			status: Status{PRErr: errors.New("not a GitHub repo")},
			reason: "couldn't check the PR: not a GitHub repo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(tt.status, tt.force)
			if got.Remove != tt.remove {
				t.Errorf("Remove = %v, want %v (reason %q)", got.Remove, tt.remove, got.Reason)
			}
			if !strings.Contains(got.Reason, tt.reason) {
				t.Errorf("Reason = %q, want it to contain %q", got.Reason, tt.reason)
			}
		})
	}
}

func TestNameFor(t *testing.T) {
	tests := []struct{ branch, prefix, want string }{
		{"backevik/swift-dancing-meadow", "backevik", "swift-dancing-meadow"},
		{"fix/cors-origins", "", "fix-cors-origins"},
		{"fix/cors-origins", "backevik", "fix-cors-origins"},
		{"plain", "", "plain"},
	}
	for _, tt := range tests {
		if got := nameFor(tt.branch, tt.prefix); got != tt.want {
			t.Errorf("nameFor(%q, %q) = %q, want %q", tt.branch, tt.prefix, got, tt.want)
		}
	}
}
