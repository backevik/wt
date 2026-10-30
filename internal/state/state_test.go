package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wt", "state.json")
	store, st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Add(Worktree{Name: "a", RepoName: "r", SessionID: "session:1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Add(Worktree{Name: "a", RepoName: "r"}); err == nil {
		t.Error("adding a duplicate succeeded")
	}
	if err := st.Add(Worktree{Name: "b", RepoName: "r"}); err != nil {
		t.Fatal(err)
	}
	st.Remove("r", "b")
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}
	store.Close()

	store, st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if len(st.Worktrees) != 1 || st.Worktrees[0].SessionID != "session:1" {
		t.Errorf("reopened state = %+v", st.Worktrees)
	}
}

func TestOpenRejectsOtherVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"version": 99}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(path); err == nil || !strings.Contains(err.Error(), "version 99") {
		t.Errorf("err = %v, want a version error", err)
	}
}
