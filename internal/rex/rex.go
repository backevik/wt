// Package rex drives Rex through its CLI.
package rex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/backevik/wt/internal/config"
)

const (
	shellFlavor = "com.superlogical.terminal.shell"
	appBinDir   = "/Applications/Rex Beta.app/Contents/Helpers/Rex Server.rex/Contents/MacOS"
	// exitUnreachable is rex's exit status when the server can't be reached.
	exitUnreachable = 4
)

// ErrUnreachable means the Rex server couldn't be reached, so nothing is
// known about its sessions. It never means a session is gone.
var ErrUnreachable = errors.New("rex server is unreachable")

// Rex runs the rex CLI.
type Rex struct {
	Bin string
}

// New finds the rex binary: the configured one, then PATH, then the Rex app.
func New(configured string) (*Rex, error) {
	if configured != "" {
		return &Rex{Bin: configured}, nil
	}
	if p, err := exec.LookPath("rex"); err == nil {
		return &Rex{Bin: p}, nil
	}
	for _, dir := range []string{os.Getenv("REX_BIN_DIR"), appBinDir} {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, "rex")
		if _, err := os.Stat(p); err == nil {
			return &Rex{Bin: p}, nil
		}
	}
	return nil, errors.New("rex not found: install Rex, or set rex_bin in the wt config")
}

func (r *Rex) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == exitUnreachable {
			return nil, ErrUnreachable
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("rex %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

// Session is a Rex session.
type Session struct {
	ID    string `json:"session_id"`
	Label string `json:"label"`
}

// Sessions lists every session on the server.
func (r *Rex) Sessions(ctx context.Context) ([]Session, error) {
	out, err := r.run(ctx, nil, "ls", "--json")
	if err != nil {
		return nil, err
	}
	var res struct {
		Sessions []Session `json:"sessions"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("parse rex ls: %w", err)
	}
	return res.Sessions, nil
}

// CreateSession creates a labelled session with the layout's tabs, each pane
// starting in dir, and returns its ID.
func (r *Rex) CreateSession(ctx context.Context, label string, layout config.Layout, dir string) (string, error) {
	req := createRequest{Label: label}
	for _, t := range layout.Tabs {
		req.Windows = append(req.Windows, window{Label: t.Title, Layout: BuildNode(t.Root, dir)})
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	out, err := r.run(ctx, body, "api", "call", "session.create", "-")
	if err != nil {
		return "", err
	}
	var res struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return "", fmt.Errorf("parse session.create: %w", err)
	}
	if res.SessionID == "" {
		return "", errors.New("session.create returned no session ID")
	}
	return res.SessionID, nil
}

// Kill destroys a session and every pane in it.
func (r *Rex) Kill(ctx context.Context, sessionID string) error {
	_, err := r.run(ctx, nil, "kill", sessionID)
	return err
}

// Select asks the Rex app to show a session. It works when exactly one app
// is connected, or when REX_CLIENT names one.
func (r *Rex) Select(ctx context.Context, sessionID string) error {
	_, err := r.run(ctx, nil, "do", "session.select", "session_id="+sessionID)
	return err
}

type createRequest struct {
	Label   string   `json:"label"`
	Windows []window `json:"initial_windows"`
}

type window struct {
	Label  string `json:"window_label,omitempty"`
	Layout Node   `json:"layout"`
}

// Node is a layout node in the shape session.create takes.
type Node struct {
	Block *Block `json:"block,omitempty"`
	Split *Split `json:"split,omitempty"`
}

// Block is a pane.
type Block struct {
	Flavor  string       `json:"flavor"`
	Label   string       `json:"label,omitempty"`
	Options BlockOptions `json:"options"`
}

// BlockOptions are a terminal pane's options.
type BlockOptions struct {
	Cwd          string   `json:"cwd"`
	Command      []string `json:"command,omitempty"`
	InitialInput string   `json:"initial_input,omitempty"`
}

// Split divides its space between two nodes.
type Split struct {
	Direction string  `json:"direction"`
	Ratio     float64 `json:"ratio,omitempty"`
	Before    Node    `json:"before"`
	After     Node    `json:"after"`
}

// BuildNode converts a configured layout node into Rex's layout shape, with
// pane directories resolved against dir.
func BuildNode(n config.Node, dir string) Node {
	if n.Split != "" {
		return Node{Split: &Split{
			Direction: n.Split,
			Ratio:     n.Ratio,
			Before:    BuildNode(*n.Before, dir),
			After:     BuildNode(*n.After, dir),
		}}
	}
	opts := BlockOptions{Cwd: filepath.Join(dir, n.Dir), Command: n.Command}
	if n.Input != "" {
		opts.InitialInput = strings.TrimRight(n.Input, "\n") + "\n"
	}
	return Node{Block: &Block{Flavor: shellFlavor, Label: n.Pane, Options: opts}}
}
