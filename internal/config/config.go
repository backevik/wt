// Package config loads wt's configuration file: where worktrees live, which
// repos wt knows by name, and the Rex layout each repo's sessions get.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Config is the parsed contents of config.yaml.
type Config struct {
	// WorktreesDir holds every worktree wt creates, as <dir>/<repo>/<name>.
	WorktreesDir string `yaml:"worktrees_dir"`
	// BranchPrefix starts random branch names. Empty uses the GitHub username.
	BranchPrefix string `yaml:"branch_prefix"`
	// DefaultRepo is used when a command runs outside a repo, such as from Rex or Raycast.
	DefaultRepo string `yaml:"default_repo"`
	// RexBin is the rex executable. Empty finds it on PATH or in the Rex app.
	RexBin  string            `yaml:"rex_bin"`
	Repos   []Repo            `yaml:"repos"`
	Layouts map[string]Layout `yaml:"layouts"`
}

// Repo maps a repository to a name and a layout.
type Repo struct {
	Name   string `yaml:"name"`
	Path   string `yaml:"path"`
	Layout string `yaml:"layout"`
}

// Layout is the set of tabs a worktree's Rex session opens with.
type Layout struct {
	Tabs []Tab `yaml:"tabs"`
}

// Tab is one Rex window in the session.
type Tab struct {
	Title string `yaml:"title"`
	Root  Node   `yaml:"root"`
}

// Node is either a pane (Pane is set) or a split of two nodes (Split is set).
type Node struct {
	// Pane labels a terminal pane.
	Pane string `yaml:"pane"`
	// Command runs instead of a shell. The pane closes when it exits.
	Command []string `yaml:"command"`
	// Input is typed into the pane's shell and run, so the shell stays after it exits.
	Input string `yaml:"input"`
	// Dir is the pane's directory, relative to the worktree.
	Dir string `yaml:"dir"`

	// Split is "horizontal" (before on the left) or "vertical" (before on top).
	Split  string  `yaml:"split"`
	Ratio  float64 `yaml:"ratio"`
	Before *Node   `yaml:"before"`
	After  *Node   `yaml:"after"`
}

// DefaultLayout is used for repos without a configured layout: one shell tab.
var DefaultLayout = Layout{Tabs: []Tab{{Title: "shell", Root: Node{Pane: "shell"}}}}

// Path returns the config file's location, honouring XDG_CONFIG_HOME.
func Path() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "wt", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "wt", "config.yaml"), nil
}

// Load reads and validates the config file. A missing file gives the defaults.
func Load(path string) (*Config, error) {
	cfg := &Config{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	if err := cfg.normalize(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func (c *Config) normalize() error {
	if c.WorktreesDir == "" {
		c.WorktreesDir = "~/worktrees"
	}
	var err error
	if c.WorktreesDir, err = ExpandHome(c.WorktreesDir); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i := range c.Repos {
		r := &c.Repos[i]
		if r.Path == "" {
			return fmt.Errorf("repos[%d]: path is required", i)
		}
		if r.Path, err = ExpandHome(r.Path); err != nil {
			return err
		}
		r.Path = filepath.Clean(r.Path)
		if r.Name == "" {
			r.Name = filepath.Base(r.Path)
		}
		if seen[r.Name] {
			return fmt.Errorf("repos[%d]: name %q is used twice", i, r.Name)
		}
		seen[r.Name] = true
		if r.Layout != "" {
			if _, ok := c.Layouts[r.Layout]; !ok {
				return fmt.Errorf("repo %q: layout %q is not defined", r.Name, r.Layout)
			}
		}
	}
	for name, l := range c.Layouts {
		if len(l.Tabs) == 0 {
			return fmt.Errorf("layout %q: needs at least one tab", name)
		}
		for i, t := range l.Tabs {
			if err := t.Root.validate(); err != nil {
				return fmt.Errorf("layout %q, tab %d: %w", name, i+1, err)
			}
		}
	}
	return nil
}

func (n *Node) validate() error {
	isSplit := n.Split != "" || n.Before != nil || n.After != nil
	switch {
	case isSplit && (n.Pane != "" || len(n.Command) > 0 || n.Input != "" || n.Dir != ""):
		return errors.New("a node is either a pane or a split, not both")
	case isSplit:
		if n.Split != "horizontal" && n.Split != "vertical" {
			return fmt.Errorf("split must be horizontal or vertical, not %q", n.Split)
		}
		if n.Before == nil || n.After == nil {
			return errors.New("a split needs both before and after")
		}
		if n.Ratio < 0 || n.Ratio >= 1 {
			return fmt.Errorf("ratio must be between 0 and 1, not %v", n.Ratio)
		}
		if err := n.Before.validate(); err != nil {
			return err
		}
		return n.After.validate()
	case len(n.Command) > 0 && n.Input != "":
		return fmt.Errorf("pane %q: set command or input, not both", n.Pane)
	case filepath.IsAbs(n.Dir):
		return fmt.Errorf("pane %q: dir must be relative to the worktree", n.Pane)
	}
	return nil
}

// RepoByPath returns the configured repo at path, if any.
func (c *Config) RepoByPath(path string) (Repo, bool) {
	path = filepath.Clean(path)
	for _, r := range c.Repos {
		if r.Path == path {
			return r, true
		}
	}
	return Repo{}, false
}

// RepoByName returns the configured repo called name, if any.
func (c *Config) RepoByName(name string) (Repo, bool) {
	for _, r := range c.Repos {
		if r.Name == name {
			return r, true
		}
	}
	return Repo{}, false
}

// LayoutFor returns the layout for a repo, or DefaultLayout.
func (c *Config) LayoutFor(r Repo) Layout {
	if l, ok := c.Layouts[r.Layout]; ok {
		return l
	}
	return DefaultLayout
}

// ExpandHome replaces a leading ~ with the home directory.
func ExpandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
}
