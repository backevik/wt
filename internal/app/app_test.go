package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/backevik/wt/internal/config"
	"github.com/backevik/wt/internal/github"
	"github.com/backevik/wt/internal/rex"
	"github.com/backevik/wt/internal/state"
)

// fakeRex keeps sessions as lines in a file. "unreachable" makes every call
// exit 4, as rex does when the server is down.
const fakeRex = `#!/bin/sh
d="$FAKE_REX_DIR"
[ -e "$d/unreachable" ] && exit 4
touch "$d/sessions"
case "$1" in
ls)
  printf '{"sessions":['
  sep=""
  while read -r id; do printf '%s{"session_id":"%s","label":"x"}' "$sep" "$id"; sep=","; done < "$d/sessions"
  printf ']}\n' ;;
api)
  n=$(( $(cat "$d/counter" 2>/dev/null || echo 0) + 1 )); echo "$n" > "$d/counter"
  cat > "$d/create-$n.json"
  echo "session:$n" >> "$d/sessions"
  printf '{"session_id":"session:%s"}\n' "$n" ;;
kill)
  grep -vx "$2" "$d/sessions" > "$d/sessions.tmp"; mv "$d/sessions.tmp" "$d/sessions" ;;
do)
  echo "$*" >> "$d/selects" ;;
esac
`

// fakeGH answers pr list from <dir>/<branch with / as _>.json.
const fakeGH = `#!/bin/sh
case "$1" in
api) echo tester ;;
pr)
  while [ $# -gt 0 ]; do [ "$1" = --head ] && b="$2"; shift; done
  f="$FAKE_GH_DIR/$(echo "$b" | tr / _).json"
  if [ -e "$f" ]; then cat "$f"; else echo '[]'; fi ;;
esac
`

type env struct {
	t      *testing.T
	app    *App
	origin string
	repo   string
	rexDir string
	ghDir  string
	out    *bytes.Buffer
}

func newEnv(t *testing.T) *env {
	t.Helper()
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t",
		"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1",
	} {
		t.Setenv(k, v)
	}
	root := t.TempDir()
	e := &env{
		t:      t,
		origin: filepath.Join(root, "origin.git"),
		repo:   filepath.Join(root, "repo"),
		rexDir: filepath.Join(root, "rex"),
		ghDir:  filepath.Join(root, "gh"),
		out:    &bytes.Buffer{},
	}
	for _, d := range []string{e.rexDir, e.ghDir, filepath.Join(root, "bin")} {
		must(t, os.MkdirAll(d, 0o755))
	}
	t.Setenv("FAKE_REX_DIR", e.rexDir)
	t.Setenv("FAKE_GH_DIR", e.ghDir)
	rexBin := filepath.Join(root, "bin", "rex")
	ghBin := filepath.Join(root, "bin", "gh")
	must(t, os.WriteFile(rexBin, []byte(fakeRex), 0o755))
	must(t, os.WriteFile(ghBin, []byte(fakeGH), 0o755))

	e.git(root, "init", "--quiet", "--bare", "-b", "main", e.origin)
	e.git(root, "clone", "--quiet", e.origin, e.repo)
	e.commit(e.repo, "README", "hello")
	e.git(e.repo, "push", "--quiet", "origin", "HEAD:main")
	e.git(e.repo, "remote", "set-head", "origin", "main")

	e.app = &App{
		Cfg: &config.Config{
			WorktreesDir: filepath.Join(root, "worktrees"),
			BranchPrefix: "tester",
		},
		GH:        github.GitHub{Bin: ghBin},
		Rex:       &rex.Rex{Bin: rexBin},
		StatePath: filepath.Join(root, "state", "state.json"),
		Out:       e.out,
		Err:       e.out,
	}
	return e
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (e *env) git(dir string, args ...string) string {
	e.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		e.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (e *env) commit(dir, file, content string) string {
	e.t.Helper()
	must(e.t, os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644))
	e.git(dir, "add", file)
	e.git(dir, "commit", "--quiet", "-m", file)
	return e.git(dir, "rev-parse", "HEAD")
}

func (e *env) new(branch string, noSession bool) state.Worktree {
	e.t.Helper()
	must(e.t, e.app.New(context.Background(), NewOptions{Branch: branch, Repo: e.repo, NoSession: noSession}))
	st := e.state()
	return st.Worktrees[len(st.Worktrees)-1]
}

func (e *env) state() *state.State {
	e.t.Helper()
	store, st, err := state.Open(e.app.StatePath)
	must(e.t, err)
	store.Close()
	return st
}

func (e *env) find(name string) (state.Worktree, bool) {
	e.t.Helper()
	w, ok := e.state().Find("repo", name)
	if !ok {
		return state.Worktree{}, false
	}
	return *w, true
}

func (e *env) closeSessionsInRex() {
	must(e.t, os.WriteFile(filepath.Join(e.rexDir, "sessions"), nil, 0o644))
}

func (e *env) setPR(branch string, pr github.PR) {
	data, err := json.Marshal([]github.PR{pr})
	must(e.t, err)
	must(e.t, os.WriteFile(filepath.Join(e.ghDir, strings.ReplaceAll(branch, "/", "_")+".json"), data, 0o644))
}

func TestNewCreatesWorktreeAndSession(t *testing.T) {
	e := newEnv(t)
	w := e.new("feat/x", false)

	if w.Name != "feat-x" || w.Branch != "feat/x" || w.SessionID != "session:1" {
		t.Errorf("recorded %+v", w)
	}
	if got := e.git(w.Path, "branch", "--show-current"); got != "feat/x" {
		t.Errorf("worktree is on %q", got)
	}
	if got, want := w.BaseCommit, e.git(e.repo, "rev-parse", "origin/main"); got != want {
		t.Errorf("BaseCommit = %s, want origin/main %s", got, want)
	}
	if upstream, err := exec.Command("git", "-C", w.Path, "rev-parse", "--abbrev-ref", "@{upstream}").Output(); err == nil {
		t.Errorf("branch tracks %s, want no upstream until pushed", upstream)
	}

	req, err := os.ReadFile(filepath.Join(e.rexDir, "create-1.json"))
	must(t, err)
	for _, want := range []string{`"label":"repo/feat-x"`, `"cwd":"` + w.Path + `"`} {
		if !strings.Contains(string(req), want) {
			t.Errorf("session.create request %s lacks %s", req, want)
		}
	}
	selects, _ := os.ReadFile(filepath.Join(e.rexDir, "selects"))
	if !strings.Contains(string(selects), "session_id=session:1") {
		t.Errorf("app not switched to the session, selects: %q", selects)
	}
}

func TestNewRandomBranch(t *testing.T) {
	e := newEnv(t)
	w := e.new("", true)
	if !strings.HasPrefix(w.Branch, "tester/") || strings.Count(w.Name, "-") != 2 || strings.Contains(w.Name, "/") {
		t.Errorf("random branch %q, name %q", w.Branch, w.Name)
	}
}

func TestClosedSessionIsClearedAndRecreated(t *testing.T) {
	e := newEnv(t)
	e.new("feat/x", false)
	e.closeSessionsInRex()

	must(t, e.app.Doctor(context.Background()))
	if !strings.Contains(e.out.String(), "Rex session was closed") {
		t.Errorf("doctor output lacks the finding:\n%s", e.out)
	}
	if w, _ := e.find("feat-x"); w.SessionID != "" {
		t.Errorf("SessionID = %q, want it cleared", w.SessionID)
	}

	must(t, e.app.Open(context.Background(), "feat"))
	if w, _ := e.find("feat-x"); w.SessionID != "session:2" {
		t.Errorf("SessionID = %q, want the new session:2", w.SessionID)
	}
}

func TestUnreachableRexKeepsSession(t *testing.T) {
	e := newEnv(t)
	e.new("feat/x", false)
	must(t, os.WriteFile(filepath.Join(e.rexDir, "unreachable"), nil, 0o644))

	must(t, e.app.Doctor(context.Background()))
	if w, _ := e.find("feat-x"); w.SessionID != "session:1" {
		t.Errorf("SessionID = %q, want session:1 kept while Rex is unreachable", w.SessionID)
	}
}

func TestSwitchedBranchIsRecorded(t *testing.T) {
	e := newEnv(t)
	w := e.new("feat/x", true)
	e.git(w.Path, "checkout", "--quiet", "-b", "feat/y")

	must(t, e.app.List(context.Background(), ListOptions{NoPR: true}))
	if got, _ := e.find("feat-x"); got.Branch != "feat/y" {
		t.Errorf("Branch = %q, want feat/y", got.Branch)
	}
}

func TestClean(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// Merged into main without a PR.
	merged := e.new("merged", true)
	e.commit(merged.Path, "m", "m")
	e.git(merged.Path, "push", "--quiet", "origin", "HEAD:main")

	// Squash-merged PR whose remote branch is gone: unpushed, but GitHub has the head.
	squashed := e.new("squashed", true)
	e.setPR("squashed", github.PR{Number: 7, State: github.StateMerged, HeadOID: e.commit(squashed.Path, "s", "s")})

	// Finished, but with local changes.
	dirty := e.new("dirty", true)
	e.setPR("dirty", github.PR{Number: 8, State: github.StateClosed})
	must(t, os.WriteFile(filepath.Join(dirty.Path, "scratch"), []byte("x"), 0o644))

	// Kept: work in progress, unpushed commits, fresh branch, open session.
	wip := e.new("wip", true)
	e.setPR("wip", github.PR{Number: 9, State: github.StateOpen})
	unpushed := e.new("unpushed", true)
	e.commit(unpushed.Path, "u", "u")
	e.new("fresh", true)
	e.new("open", false)
	e.setPR("open", github.PR{Number: 10, State: github.StateMerged})
	e.setPR("unpushed", github.PR{Number: 11, State: github.StateMerged, HeadOID: "elsewhere"})

	must(t, e.app.Clean(ctx, CleanOptions{DryRun: true}))
	if len(e.state().Worktrees) != 7 {
		t.Fatalf("dry run removed worktrees:\n%s", e.out)
	}

	e.out.Reset()
	must(t, e.app.Clean(ctx, CleanOptions{}))
	for name, want := range map[string]bool{
		"merged": false, "squashed": false, "dirty": true, "wip": true, "unpushed": true, "fresh": true, "open": true,
	} {
		if _, ok := e.find(name); ok != want {
			t.Errorf("%s kept = %v, want %v\n%s", name, ok, want, e.out)
		}
	}
	for _, w := range []state.Worktree{merged, squashed} {
		if _, err := os.Stat(w.Path); err == nil {
			t.Errorf("%s folder still exists", w.Name)
		}
		if out, _ := exec.Command("git", "-C", e.repo, "branch", "--list", w.Branch).Output(); len(out) != 0 {
			t.Errorf("branch %s still exists", w.Branch)
		}
	}
	if _, err := os.Stat(wip.Path); err != nil {
		t.Errorf("wip folder removed")
	}

	e.out.Reset()
	must(t, e.app.Clean(ctx, CleanOptions{Force: true}))
	if _, ok := e.find("dirty"); ok {
		t.Errorf("dirty kept with --force:\n%s", e.out)
	}
}

func TestRemove(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	w := e.new("feat/x", false)
	must(t, os.WriteFile(filepath.Join(w.Path, "scratch"), []byte("x"), 0o644))

	err := e.app.Remove(ctx, "feat-x", false)
	if err == nil || !strings.Contains(err.Error(), "scratch") {
		t.Fatalf("err = %v, want a refusal listing scratch", err)
	}
	must(t, e.app.Remove(ctx, "feat-x", true))
	if _, ok := e.find("feat-x"); ok {
		t.Error("still recorded")
	}
	if _, err := os.Stat(w.Path); err == nil {
		t.Error("folder still exists")
	}
	if sessions, _ := os.ReadFile(filepath.Join(e.rexDir, "sessions")); len(bytes.TrimSpace(sessions)) != 0 {
		t.Errorf("session not killed: %q", sessions)
	}
	// Not finished, so the branch stays.
	if out := e.git(e.repo, "branch", "--list", "feat/x"); out == "" {
		t.Error("unfinished branch was deleted")
	}
}
