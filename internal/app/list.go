package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/backevik/wt/internal/state"
)

// pick finds the worktree a query names: an exact name or repo/name, then a
// unique substring, then an fzf picker over the remaining candidates.
func (a *App) pick(ctx context.Context, st *state.State, query string) (*state.Worktree, error) {
	if len(st.Worktrees) == 0 {
		return nil, errors.New("no worktrees: create one with wt new")
	}
	var matches []int
	for i, w := range st.Worktrees {
		if query == w.Name || query == w.Key() {
			return &st.Worktrees[i], nil
		}
		if strings.Contains(w.Key(), query) {
			matches = append(matches, i)
		}
	}
	if len(matches) == 1 {
		return &st.Worktrees[matches[0]], nil
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no worktree matches %q", query)
	}

	var keys []string
	for _, i := range matches {
		keys = append(keys, st.Worktrees[i].Key())
	}
	choice, err := fzf(ctx, keys, query)
	if err != nil {
		return nil, err
	}
	for _, i := range matches {
		if st.Worktrees[i].Key() == choice {
			return &st.Worktrees[i], nil
		}
	}
	return nil, fmt.Errorf("no worktree named %q", choice)
}

func fzf(ctx context.Context, items []string, query string) (string, error) {
	if _, err := exec.LookPath("fzf"); err != nil {
		return "", fmt.Errorf("several worktrees match, name one of:\n  %s", strings.Join(items, "\n  "))
	}
	cmd := exec.CommandContext(ctx, "fzf", "--height=40%", "--reverse", "--select-1", "--exit-0", "--query", query)
	cmd.Stdin = strings.NewReader(strings.Join(items, "\n"))
	cmd.Stderr = os.Stderr
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", errors.New("no worktree picked")
	}
	return strings.TrimSpace(out.String()), nil
}

// ListOptions are wt ls's options.
type ListOptions struct {
	JSON bool
	// NoPR skips the GitHub lookups, which are the slow part.
	NoPR bool
}

type listItem struct {
	Name     string `json:"name"`
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Path     string `json:"path"`
	Session  string `json:"session_id,omitempty"`
	Changes  int    `json:"changes"`
	Unpushed int    `json:"unpushed"`
	PR       string `json:"pr,omitempty"`
	PRURL    string `json:"pr_url,omitempty"`
	Merged   bool   `json:"merged_into_main"`
	Error    string `json:"error,omitempty"`
}

// List prints every worktree and its status.
func (a *App) List(ctx context.Context, opts ListOptions) error {
	s, _, err := a.open(ctx)
	if err != nil {
		return err
	}
	defer s.close()
	statuses := a.gather(ctx, s, s.state.Worktrees, !opts.NoPR)

	items := make([]listItem, 0, len(statuses))
	for _, st := range statuses {
		w := st.Worktree
		it := listItem{
			Name: w.Name, Repo: w.RepoName, Branch: w.Branch, Path: w.Path,
			Session: w.SessionID, Changes: len(st.Changes), Unpushed: st.Unpushed,
			Merged: st.MergedIntoMain,
		}
		if st.PR != nil {
			it.PR = fmt.Sprintf("#%d %s", st.PR.Number, strings.ToLower(st.PR.State))
			it.PRURL = st.PR.URL
		}
		if st.Err != nil {
			it.Error = st.Err.Error()
		} else if st.PRErr != nil {
			it.Error = "PR lookup failed: " + st.PRErr.Error()
		}
		items = append(items, it)
	}

	if opts.JSON {
		enc := json.NewEncoder(a.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"worktrees": items})
	}
	if len(items) == 0 {
		fmt.Fprintln(a.Out, "No worktrees. Create one with wt new.")
		return nil
	}
	tw := tabwriter.NewWriter(a.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "WORKTREE\tBRANCH\tSESSION\tCHANGES\tUNPUSHED\tPR")
	for _, it := range items {
		session := "-"
		if it.Session != "" {
			session = "open"
		}
		pr := it.PR
		switch {
		case pr == "" && it.Merged:
			pr = "merged into main"
		case pr == "" && opts.NoPR:
			pr = "?"
		case pr == "":
			pr = "-"
		}
		if it.Error != "" {
			pr += " (error: " + it.Error + ")"
		}
		fmt.Fprintf(tw, "%s/%s\t%s\t%s\t%d\t%d\t%s\n", it.Repo, it.Name, it.Branch, session, it.Changes, it.Unpushed, pr)
	}
	return tw.Flush()
}
