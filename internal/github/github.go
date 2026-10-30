// Package github looks up pull requests with the gh CLI.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PR states as gh reports them.
const (
	StateOpen   = "OPEN"
	StateMerged = "MERGED"
	StateClosed = "CLOSED"
)

// PR is the newest pull request for a branch.
type PR struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	// HeadOID is the commit the PR's branch pointed at, last time GitHub saw it.
	HeadOID string `json:"headRefOid"`
	URL     string `json:"url"`
}

// Done reports whether the PR is merged or closed.
func (p *PR) Done() bool {
	return p != nil && (p.State == StateMerged || p.State == StateClosed)
}

// GitHub runs gh.
type GitHub struct {
	Bin string
}

func (g GitHub) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	bin := g.Bin
	if bin == "" {
		bin = "gh"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

// PRForBranch returns the newest PR whose head is branch, or nil if there is none.
func (g GitHub) PRForBranch(ctx context.Context, repo, branch string) (*PR, error) {
	out, err := g.run(ctx, repo, "pr", "list", "--state", "all", "--head", branch,
		"--limit", "1", "--json", "number,state,headRefOid,url")
	if err != nil {
		return nil, err
	}
	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list: %w", err)
	}
	if len(prs) == 0 {
		return nil, nil
	}
	return &prs[0], nil
}

// Username returns the signed-in GitHub user's login.
func (g GitHub) Username(ctx context.Context) (string, error) {
	out, err := g.run(ctx, "", "api", "user", "--jq", ".login")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
