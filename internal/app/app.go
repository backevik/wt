// Package app implements wt's commands.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/backevik/wt/internal/config"
	"github.com/backevik/wt/internal/git"
	"github.com/backevik/wt/internal/github"
	"github.com/backevik/wt/internal/names"
	"github.com/backevik/wt/internal/rex"
	"github.com/backevik/wt/internal/state"
)

// App holds wt's dependencies.
type App struct {
	Cfg       *config.Config
	Git       git.Git
	GH        github.GitHub
	Rex       *rex.Rex
	StatePath string
	Out       io.Writer
	Err       io.Writer
}

// session is an open state file, checked for staleness.
type session struct {
	store *state.Store
	state *state.State
	// openSessions is nil when Rex couldn't be reached.
	openSessions map[string]bool
}

func (s *session) close() { s.store.Close() }

func (s *session) save() error { return s.store.Save(s.state) }

// open locks the state file and fixes what has gone stale since the last
// command: sessions that were closed and branches that were switched.
func (a *App) open(ctx context.Context) (*session, []Finding, error) {
	store, st, err := state.Open(a.StatePath)
	if err != nil {
		return nil, nil, err
	}
	s := &session{store: store, state: st}
	findings, changed := a.reconcile(ctx, s)
	if changed {
		if err := s.save(); err != nil {
			s.close()
			return nil, nil, err
		}
	}
	return s, findings, nil
}

// Finding is something stale that reconcile found.
type Finding struct {
	Worktree string
	Message  string
	// Fixed is true when reconcile corrected the state file.
	Fixed bool
}

// reconcile compares the state file with Rex and git. It fixes the safe
// cases and reports the rest. When Rex is unreachable it leaves session IDs
// alone, since an unreachable server says nothing about whether they exist.
func (a *App) reconcile(ctx context.Context, s *session) ([]Finding, bool) {
	var findings []Finding
	changed := false

	sessions, err := a.Rex.Sessions(ctx)
	switch {
	case err != nil:
		findings = append(findings, Finding{Message: "can't check Rex sessions: " + err.Error()})
	default:
		s.openSessions = make(map[string]bool, len(sessions))
		for _, rs := range sessions {
			s.openSessions[rs.ID] = true
		}
	}

	for i := range s.state.Worktrees {
		w := &s.state.Worktrees[i]
		if s.openSessions != nil && w.SessionID != "" && !s.openSessions[w.SessionID] {
			findings = append(findings, Finding{Worktree: w.Key(), Message: "Rex session was closed, cleared it", Fixed: true})
			w.SessionID = ""
			changed = true
		}
		if _, err := os.Stat(w.Path); err != nil {
			findings = append(findings, Finding{Worktree: w.Key(), Message: "folder is missing: " + w.Path})
			continue
		}
		branch, err := a.Git.CurrentBranch(ctx, w.Path)
		switch {
		case err != nil:
			findings = append(findings, Finding{Worktree: w.Key(), Message: "can't read branch: " + err.Error()})
		case branch == "":
			findings = append(findings, Finding{Worktree: w.Key(), Message: "HEAD is detached, still recorded as " + w.Branch})
		case branch != w.Branch:
			findings = append(findings, Finding{Worktree: w.Key(), Message: fmt.Sprintf("branch changed from %s to %s, updated it", w.Branch, branch), Fixed: true})
			w.Branch = branch
			if base, err := a.mergeBase(ctx, w.Repo, branch); err == nil {
				w.BaseCommit = base
			}
			changed = true
		}
	}
	return findings, changed
}

func (a *App) mergeBase(ctx context.Context, repo, branch string) (string, error) {
	return a.Git.MergeBase(ctx, repo, branch, a.Git.DefaultBranch(ctx, repo))
}

// resolveRepo finds the repo a command acts on: the --repo flag (a
// configured name or a path), the current directory, or default_repo.
func (a *App) resolveRepo(ctx context.Context, flag string) (config.Repo, error) {
	target := flag
	if target == "" {
		if cwd, err := os.Getwd(); err == nil {
			if repo, err := a.repoAt(ctx, cwd); err == nil {
				return repo, nil
			}
		}
		target = a.Cfg.DefaultRepo
		if target == "" {
			return config.Repo{}, errors.New("not in a git repo: pass --repo or set default_repo in the config")
		}
	}
	if r, ok := a.Cfg.RepoByName(target); ok {
		return r, nil
	}
	path, err := config.ExpandHome(target)
	if err != nil {
		return config.Repo{}, err
	}
	return a.repoAt(ctx, path)
}

func (a *App) repoAt(ctx context.Context, dir string) (config.Repo, error) {
	main, err := a.Git.MainRepo(ctx, dir)
	if err != nil {
		return config.Repo{}, err
	}
	if r, ok := a.Cfg.RepoByPath(main); ok {
		return r, nil
	}
	return config.Repo{Name: filepath.Base(main), Path: main}, nil
}

// NewOptions are wt new's options.
type NewOptions struct {
	Branch string
	Name   string
	Repo   string
	// NoSession skips creating a Rex session.
	NoSession bool
}

// New creates a worktree on a new branch from the default branch, records
// it, and opens a Rex session for it.
func (a *App) New(ctx context.Context, opts NewOptions) error {
	repo, err := a.resolveRepo(ctx, opts.Repo)
	if err != nil {
		return err
	}
	s, _, err := a.open(ctx)
	if err != nil {
		return err
	}
	defer s.close()

	fmt.Fprintf(a.Err, "Fetching %s...\n", repo.Name)
	if err := a.Git.Fetch(ctx, repo.Path); err != nil {
		return err
	}

	branch := opts.Branch
	prefix := ""
	if branch == "" {
		if prefix, err = a.branchPrefix(ctx); err != nil {
			return err
		}
		if branch, err = a.randomBranch(ctx, repo.Path, prefix); err != nil {
			return err
		}
	}
	name := opts.Name
	if name == "" {
		name = nameFor(branch, prefix)
	}
	if _, ok := s.state.Find(repo.Name, name); ok {
		return fmt.Errorf("worktree %s/%s already exists: use wt open, or pass --name", repo.Name, name)
	}
	path := filepath.Join(a.Cfg.WorktreesDir, repo.Name, name)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}

	base := a.Git.DefaultBranch(ctx, repo.Path)
	exists, err := a.Git.BranchExists(ctx, repo.Path, branch)
	if err != nil {
		return err
	}
	var baseCommit string
	if exists {
		if err := a.Git.AddWorktree(ctx, repo.Path, path, branch, ""); err != nil {
			return err
		}
		baseCommit, err = a.Git.MergeBase(ctx, repo.Path, branch, base)
	} else {
		if baseCommit, err = a.Git.RevParse(ctx, repo.Path, base); err != nil {
			return fmt.Errorf("resolve %s: %w", base, err)
		}
		err = a.Git.AddWorktree(ctx, repo.Path, path, branch, base)
	}
	if err != nil {
		return err
	}

	w := state.Worktree{
		Name:       name,
		Repo:       repo.Path,
		RepoName:   repo.Name,
		Path:       path,
		Branch:     branch,
		BaseCommit: baseCommit,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.state.Add(w); err != nil {
		return err
	}
	if err := s.save(); err != nil {
		return err
	}
	verb := "Created"
	if exists {
		verb = "Checked out"
	}
	fmt.Fprintf(a.Out, "%s %s at %s\n", verb, branch, path)

	if opts.NoSession {
		return nil
	}
	saved, _ := s.state.Find(repo.Name, name)
	return a.openSession(ctx, s, saved, repo)
}

// openSession switches the Rex app to the worktree's session, creating it
// with the repo's layout if none is open.
func (a *App) openSession(ctx context.Context, s *session, w *state.Worktree, repo config.Repo) error {
	if w.SessionID == "" {
		if _, err := os.Stat(w.Path); err != nil {
			return fmt.Errorf("worktree folder is missing: %s", w.Path)
		}
		id, err := a.Rex.CreateSession(ctx, w.Key(), a.Cfg.LayoutFor(repo), w.Path)
		if err != nil {
			return fmt.Errorf("create Rex session: %w (the worktree is kept; retry with wt open %s)", err, w.Name)
		}
		w.SessionID = id
		if err := s.save(); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "Opened Rex session %s\n", w.Key())
	}
	if err := a.Rex.Select(ctx, w.SessionID); err != nil {
		fmt.Fprintf(a.Err, "warning: couldn't switch the Rex app to the session: %v\n", err)
	}
	return nil
}

func (a *App) branchPrefix(ctx context.Context) (string, error) {
	if a.Cfg.BranchPrefix != "" {
		return a.Cfg.BranchPrefix, nil
	}
	user, err := a.GH.Username(ctx)
	if err != nil {
		return "", fmt.Errorf("get GitHub username for the branch prefix (or set branch_prefix): %w", err)
	}
	return user, nil
}

func (a *App) randomBranch(ctx context.Context, repo, prefix string) (string, error) {
	for range 10 {
		branch := prefix + "/" + names.Random()
		exists, err := a.Git.BranchExists(ctx, repo, branch)
		if err != nil {
			return "", err
		}
		if !exists {
			return branch, nil
		}
	}
	return "", errors.New("couldn't find an unused random branch name")
}

// nameFor turns a branch into a folder name: the prefix is dropped and
// slashes become dashes.
func nameFor(branch, prefix string) string {
	if prefix != "" {
		branch = strings.TrimPrefix(branch, prefix+"/")
	}
	return strings.ReplaceAll(branch, "/", "-")
}

// Open switches to a worktree's Rex session, recreating it if it was closed.
func (a *App) Open(ctx context.Context, query string) error {
	s, _, err := a.open(ctx)
	if err != nil {
		return err
	}
	defer s.close()
	w, err := a.pick(ctx, s.state, query)
	if err != nil {
		return err
	}
	repo := a.repoFor(*w)
	return a.openSession(ctx, s, w, repo)
}

func (a *App) repoFor(w state.Worktree) config.Repo {
	if r, ok := a.Cfg.RepoByPath(w.Repo); ok {
		return r
	}
	return config.Repo{Name: w.RepoName, Path: w.Repo}
}

// gather reads the status of each worktree concurrently.
func (a *App) gather(ctx context.Context, s *session, ws []state.Worktree, withPR bool) []Status {
	out := make([]Status, len(ws))
	var wg sync.WaitGroup
	for i, w := range ws {
		wg.Go(func() { out[i] = a.status(ctx, s, w, withPR) })
	}
	wg.Wait()
	return out
}

func (a *App) status(ctx context.Context, s *session, w state.Worktree, withPR bool) Status {
	st := Status{Worktree: w, SessionOpen: w.SessionID != ""}
	if s.openSessions == nil && w.SessionID != "" {
		// Rex is unreachable, so assume the recorded session is still open.
		st.SessionOpen = true
	}
	var errs []error
	var err error
	if st.Head, err = a.Git.RevParse(ctx, w.Repo, "refs/heads/"+w.Branch); err != nil {
		return Status{Worktree: w, SessionOpen: st.SessionOpen, Err: fmt.Errorf("branch %s: %w", w.Branch, err)}
	}
	if _, statErr := os.Stat(w.Path); statErr == nil {
		if st.Changes, err = a.Git.Changes(ctx, w.Path); err != nil {
			errs = append(errs, err)
		}
	}
	if st.Unpushed, err = a.Git.Unpushed(ctx, w.Repo, w.Branch); err != nil {
		errs = append(errs, err)
	}
	if st.Head != w.BaseCommit {
		main := a.Git.DefaultBranch(ctx, w.Repo)
		if st.MergedIntoMain, err = a.Git.IsAncestor(ctx, w.Repo, st.Head, main); err != nil {
			errs = append(errs, err)
		}
	}
	if withPR {
		st.PR, st.PRErr = a.GH.PRForBranch(ctx, w.Repo, w.Branch)
	}
	st.Err = errors.Join(errs...)
	return st
}

// fetchAll fetches each repo the worktrees belong to, once.
func (a *App) fetchAll(ctx context.Context, ws []state.Worktree) {
	seen := map[string]bool{}
	for _, w := range ws {
		if seen[w.Repo] {
			continue
		}
		seen[w.Repo] = true
		if err := a.Git.Fetch(ctx, w.Repo); err != nil {
			fmt.Fprintf(a.Err, "warning: fetch %s: %v\n", w.RepoName, err)
		}
	}
}

// Remove deletes a worktree, its branch when the work is finished, and its
// Rex session. Without force it refuses when there are local changes.
func (a *App) Remove(ctx context.Context, query string, force bool) error {
	s, _, err := a.open(ctx)
	if err != nil {
		return err
	}
	defer s.close()
	w, err := a.pick(ctx, s.state, query)
	if err != nil {
		return err
	}
	a.fetchAll(ctx, []state.Worktree{*w})
	st := a.status(ctx, s, *w, true)
	if len(st.Changes) > 0 && !force {
		return fmt.Errorf("%s has local changes, use --force to discard them:\n%s", w.Key(), formatChanges(st.Changes))
	}
	if st.Err != nil && !force {
		return fmt.Errorf("couldn't check %s, use --force to remove anyway: %w", w.Key(), st.Err)
	}
	if st.PRErr != nil {
		fmt.Fprintf(a.Err, "warning: couldn't check the PR, so the branch is only deleted if merged into main: %v\n", st.PRErr)
	}

	sessionID := w.SessionID
	if err := a.removeOne(ctx, s, *w, force, st.Err == nil && st.Finished() && st.WorkIsSafe()); err != nil {
		return err
	}
	// Kill the session last: wt may be running inside it.
	if sessionID != "" && st.SessionOpen {
		if err := a.Rex.Kill(ctx, sessionID); err != nil {
			fmt.Fprintf(a.Err, "warning: close Rex session: %v\n", err)
		}
	}
	return nil
}

// removeOne removes the worktree folder and state entry, and the branch when
// deleteBranch is set.
func (a *App) removeOne(ctx context.Context, s *session, w state.Worktree, force, deleteBranch bool) error {
	if _, err := os.Stat(w.Path); err == nil {
		if err := a.Git.RemoveWorktree(ctx, w.Repo, w.Path, force); err != nil {
			return err
		}
	} else if err := a.Git.PruneWorktrees(ctx, w.Repo); err != nil {
		return err
	}
	branchNote := "kept branch " + w.Branch + " (not finished)"
	if deleteBranch {
		if err := a.Git.DeleteBranch(ctx, w.Repo, w.Branch); err != nil {
			branchNote = "couldn't delete branch: " + err.Error()
		} else {
			branchNote = "deleted branch " + w.Branch
		}
	}
	s.state.Remove(w.RepoName, w.Name)
	if err := s.save(); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Removed %s, %s\n", w.Key(), branchNote)
	return nil
}

func formatChanges(changes []string) string {
	const max = 10
	shown := changes
	if len(shown) > max {
		shown = shown[:max]
	}
	out := "  " + strings.Join(shown, "\n  ")
	if len(changes) > max {
		out += fmt.Sprintf("\n  ...and %d more", len(changes)-max)
	}
	return out
}

// CleanOptions are wt clean's options.
type CleanOptions struct {
	Force  bool
	DryRun bool
}

// Clean removes every finished worktree that Decide allows.
func (a *App) Clean(ctx context.Context, opts CleanOptions) error {
	s, _, err := a.open(ctx)
	if err != nil {
		return err
	}
	defer s.close()
	ws := append([]state.Worktree(nil), s.state.Worktrees...)
	if len(ws) == 0 {
		fmt.Fprintln(a.Out, "No worktrees.")
		return nil
	}
	a.fetchAll(ctx, ws)
	statuses := a.gather(ctx, s, ws, true)

	var errs []error
	for _, st := range statuses {
		v := Decide(st, opts.Force)
		if !v.Remove {
			fmt.Fprintf(a.Out, "keep    %-40s %s\n", st.Worktree.Key(), v.Reason)
			continue
		}
		if opts.DryRun {
			fmt.Fprintf(a.Out, "remove  %-40s %s\n", st.Worktree.Key(), v.Reason)
			continue
		}
		fmt.Fprintf(a.Out, "remove  %-40s %s\n", st.Worktree.Key(), v.Reason)
		if err := a.removeOne(ctx, s, st.Worktree, opts.Force, true); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", st.Worktree.Key(), err))
		}
	}
	if opts.DryRun {
		fmt.Fprintln(a.Out, "Dry run: nothing was removed.")
	}
	return errors.Join(errs...)
}

// Doctor reports what is stale and checks wt's dependencies.
func (a *App) Doctor(ctx context.Context) error {
	s, findings, err := a.open(ctx)
	if err != nil {
		return err
	}
	defer s.close()

	cfgPath, _ := config.Path()
	if _, err := os.Stat(cfgPath); err != nil {
		fmt.Fprintf(a.Out, "config   %s not found, using defaults\n", cfgPath)
	} else {
		fmt.Fprintf(a.Out, "config   %s\n", cfgPath)
	}
	fmt.Fprintf(a.Out, "state    %s (%d worktrees)\n", a.StatePath, len(s.state.Worktrees))
	fmt.Fprintf(a.Out, "rex      %s\n", a.Rex.Bin)
	if user, err := a.GH.Username(ctx); err != nil {
		fmt.Fprintf(a.Out, "gh       not signed in: %v\n", err)
	} else {
		fmt.Fprintf(a.Out, "gh       signed in as %s\n", user)
	}

	if len(findings) == 0 {
		fmt.Fprintln(a.Out, "\nNothing is stale.")
		return nil
	}
	fmt.Fprintln(a.Out)
	for _, f := range findings {
		mark := "warn "
		if f.Fixed {
			mark = "fixed"
		}
		if f.Worktree != "" {
			fmt.Fprintf(a.Out, "%s  %s: %s\n", mark, f.Worktree, f.Message)
		} else {
			fmt.Fprintf(a.Out, "%s  %s\n", mark, f.Message)
		}
	}
	return nil
}
