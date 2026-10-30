// Package state keeps the record of worktrees wt created and the Rex session
// each one is tied to.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"time"
)

const version = 1

// Worktree is one worktree wt created.
type Worktree struct {
	// Name is the folder name under <worktrees_dir>/<repo>, unique per repo.
	Name string `json:"name"`
	// Repo is the main repository's path.
	Repo string `json:"repo"`
	// RepoName is the repo's short name, used in paths and session labels.
	RepoName string `json:"repo_name"`
	Path     string `json:"path"`
	Branch   string `json:"branch"`
	// BaseCommit is the commit the branch started at. A branch still at its
	// base has no work of its own, so being "merged into main" means nothing.
	BaseCommit string `json:"base_commit"`
	// SessionID is the Rex session's ID. Empty when no session is open.
	SessionID string    `json:"session_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Key identifies a worktree across repos.
func (w Worktree) Key() string { return w.RepoName + "/" + w.Name }

// State is the whole state file.
type State struct {
	Version   int        `json:"version"`
	Worktrees []Worktree `json:"worktrees"`
}

// Find returns the worktree with the given repo name and name.
func (s *State) Find(repoName, name string) (*Worktree, bool) {
	for i := range s.Worktrees {
		if s.Worktrees[i].RepoName == repoName && s.Worktrees[i].Name == name {
			return &s.Worktrees[i], true
		}
	}
	return nil, false
}

// Add records a new worktree.
func (s *State) Add(w Worktree) error {
	if _, ok := s.Find(w.RepoName, w.Name); ok {
		return fmt.Errorf("worktree %s is already recorded", w.Key())
	}
	s.Worktrees = append(s.Worktrees, w)
	return nil
}

// Remove forgets a worktree.
func (s *State) Remove(repoName, name string) {
	s.Worktrees = slices.DeleteFunc(s.Worktrees, func(w Worktree) bool {
		return w.RepoName == repoName && w.Name == name
	})
}

// Store reads and writes the state file. Open takes an exclusive lock, so
// two wt commands never overwrite each other's changes.
type Store struct {
	path string
	lock *os.File
}

// DefaultPath returns the state file's location, honouring XDG_STATE_HOME.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "wt", "state.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "wt", "state.json"), nil
}

// Open locks the state file and reads it. Call Close to release the lock.
func Open(path string) (*Store, *State, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		lock.Close()
		return nil, nil, fmt.Errorf("lock %s: %w", lock.Name(), err)
	}
	st := &Store{path: path, lock: lock}
	s, err := st.read()
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	return st, s, nil
}

func (st *Store) read() (*State, error) {
	data, err := os.ReadFile(st.path)
	if errors.Is(err, fs.ErrNotExist) {
		return &State{Version: version}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", st.path, err)
	}
	if s.Version != version {
		return nil, fmt.Errorf("%s has version %d, this wt reads version %d", st.path, s.Version, version)
	}
	return &s, nil
}

// Save writes the state atomically.
func (st *Store) Save(s *State) error {
	s.Version = version
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(st.path), ".state-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), st.path)
}

// Close releases the lock.
func (st *Store) Close() error {
	return st.lock.Close()
}
