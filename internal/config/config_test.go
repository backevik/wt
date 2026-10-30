package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, "worktrees"); cfg.WorktreesDir != want {
		t.Errorf("WorktreesDir = %q, want %q", cfg.WorktreesDir, want)
	}
	if got := cfg.LayoutFor(Repo{Name: "x"}); len(got.Tabs) != 1 {
		t.Errorf("default layout has %d tabs, want 1", len(got.Tabs))
	}
}

func TestLoadRepoLayout(t *testing.T) {
	cfg, err := Load(write(t, `
repos:
  - path: ~/projects/work/main
    layout: main
layouts:
  main:
    tabs:
      - title: frontend
        root:
          split: horizontal
          before: {pane: agent, input: opencode --auto}
          after:
            split: vertical
            before: {pane: cli, dir: frontend, input: pnpm run dev:init}
            after: {pane: git}
`))
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	repo, ok := cfg.RepoByPath(filepath.Join(home, "projects/work/main"))
	if !ok {
		t.Fatal("repo not found by its expanded path")
	}
	if repo.Name != "main" {
		t.Errorf("Name = %q, want main (the folder name)", repo.Name)
	}
	root := cfg.LayoutFor(repo).Tabs[0].Root
	if root.Before.Pane != "agent" || root.After.Before.Dir != "frontend" {
		t.Errorf("layout not parsed as written: %+v", root)
	}
}

func TestExampleConfigLoads(t *testing.T) {
	cfg, err := Load("../../extras/config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	repo, ok := cfg.RepoByName(cfg.DefaultRepo)
	if !ok {
		t.Fatalf("default_repo %q is not a configured repo", cfg.DefaultRepo)
	}
	if got := cfg.LayoutFor(repo).Tabs[0].Title; got != "frontend" {
		t.Errorf("first tab = %q, want frontend", got)
	}
}

func TestLoadRejects(t *testing.T) {
	tests := map[string]struct{ yaml, want string }{
		"unknown field":    {"worktree_dir: x\n", "field worktree_dir not found"},
		"undefined layout": {"repos:\n  - path: /r\n    layout: nope\n", `layout "nope" is not defined`},
		"duplicate name":   {"repos:\n  - path: /a/r\n  - path: /b/r\n", `name "r" is used twice`},
		"pane and split": {
			"layouts:\n  l:\n    tabs:\n      - root: {pane: a, split: horizontal, before: {pane: b}, after: {pane: c}}\n",
			"either a pane or a split",
		},
		"split missing side": {
			"layouts:\n  l:\n    tabs:\n      - root: {split: vertical, before: {pane: b}}\n",
			"needs both before and after",
		},
		"bad direction": {
			"layouts:\n  l:\n    tabs:\n      - root: {split: diagonal, before: {pane: b}, after: {pane: c}}\n",
			"horizontal or vertical",
		},
		"command and input": {
			"layouts:\n  l:\n    tabs:\n      - root: {pane: a, command: [ls], input: ls}\n",
			"command or input",
		},
		"absolute dir": {
			"layouts:\n  l:\n    tabs:\n      - root: {pane: a, dir: /tmp}\n",
			"relative to the worktree",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Load(write(t, tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
