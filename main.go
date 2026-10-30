// Command wt manages git worktrees, each with its own Rex session.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"

	"github.com/backevik/wt/internal/app"
	"github.com/backevik/wt/internal/config"
	"github.com/backevik/wt/internal/rex"
	"github.com/backevik/wt/internal/state"
)

const usage = `wt manages git worktrees, each with its own Rex session.

Usage:
  wt new [BRANCH] [--name NAME] [--repo REPO] [--no-session]
  wt open [QUERY]
  wt ls [--json] [--no-pr]
  wt rm [QUERY] [--force]
  wt clean [--dry-run] [--force]
  wt doctor

Commands:
  new     Create a worktree on a new branch from origin's default branch and
          open a Rex session with the repo's layout. Without BRANCH, the branch
          is <github-user>/<three random words>.
  open    Switch to a worktree's Rex session, recreating it if it was closed.
  ls      List worktrees with their session, local changes, unpushed commits
          and PR.
  rm      Remove a worktree and its session. The branch is deleted when its PR
          is merged or closed, or it is merged into main, and no commits would
          be lost. Refuses when there are local changes, unless --force.
  clean   Remove every worktree whose PR is merged or closed, or whose branch
          is merged into main. Keeps worktrees with an open session, unpushed
          commits, or local changes (unless --force).
  doctor  Check wt's setup and report stale state.

QUERY is a worktree name, repo/name, or part of one. Without a unique match,
wt opens fzf. REPO is a repo name from the config, or a path.

Every command first fixes stale state: it forgets Rex sessions that were
closed and records branches that were switched.

Config: ~/.config/wt/config.yaml. State: ~/.local/state/wt/state.json.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "wt:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stdout, usage)
		return nil
	}
	cmd, args := args[0], args[1:]

	fs := flag.NewFlagSet("wt "+cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	var (
		name      = fs.String("name", "", "folder name for the worktree")
		repo      = fs.String("repo", "", "repo name from the config, or a path")
		noSession = fs.Bool("no-session", false, "don't open a Rex session")
		force     = fs.Bool("force", false, "discard local changes")
		dryRun    = fs.Bool("dry-run", false, "show what would be removed")
		asJSON    = fs.Bool("json", false, "print JSON")
		noPR      = fs.Bool("no-pr", false, "skip GitHub PR lookups")
	)
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return fmt.Errorf("%s takes at most one argument, got %d", cmd, len(positional))
	}
	arg := ""
	if len(positional) == 1 {
		arg = positional[0]
	}

	a, err := newApp(stdout, stderr)
	if err != nil {
		return err
	}
	switch cmd {
	case "new":
		return a.New(ctx, app.NewOptions{Branch: arg, Name: *name, Repo: *repo, NoSession: *noSession})
	case "open", "go":
		return a.Open(ctx, arg)
	case "ls", "list":
		return a.List(ctx, app.ListOptions{JSON: *asJSON, NoPR: *noPR})
	case "rm", "remove":
		return a.Remove(ctx, arg, *force)
	case "clean":
		return a.Clean(ctx, app.CleanOptions{Force: *force, DryRun: *dryRun})
	case "doctor":
		return a.Doctor(ctx)
	}
	return fmt.Errorf("unknown command %q, see wt --help", cmd)
}

// parseInterspersed parses flags wherever they appear among the arguments,
// so both "wt rm foo --force" and "wt rm --force foo" work.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	if i := slices.Index(args, "--"); i >= 0 {
		args, rest = args[:i], args[i+1:]
	}
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return append(positional, rest...), nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

func newApp(stdout, stderr io.Writer) (*app.App, error) {
	cfgPath, err := config.Path()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	statePath, err := state.DefaultPath()
	if err != nil {
		return nil, err
	}
	r, err := rex.New(cfg.RexBin)
	if err != nil {
		return nil, err
	}
	return &app.App{Cfg: cfg, Rex: r, StatePath: statePath, Out: stdout, Err: stderr}, nil
}
