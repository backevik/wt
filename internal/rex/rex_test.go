package rex

import (
	"encoding/json"
	"testing"

	"github.com/backevik/wt/internal/config"
)

func TestBuildNode(t *testing.T) {
	n := config.Node{
		Split:  "horizontal",
		Ratio:  0.6,
		Before: &config.Node{Pane: "agent", Input: "claude"},
		After: &config.Node{
			Split:  "vertical",
			Before: &config.Node{Pane: "server", Dir: "web", Input: "npm run dev\n"},
			After:  &config.Node{Pane: "logs", Command: []string{"tail", "-f", "app.log"}},
		},
	}
	got, err := json.Marshal(BuildNode(n, "/w/x"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"split":{"direction":"horizontal","ratio":0.6,` +
		`"before":{"block":{"flavor":"com.superlogical.terminal.shell","label":"agent","options":{"cwd":"/w/x","initial_input":"claude\n"}}},` +
		`"after":{"split":{"direction":"vertical",` +
		`"before":{"block":{"flavor":"com.superlogical.terminal.shell","label":"server","options":{"cwd":"/w/x/web","initial_input":"npm run dev\n"}}},` +
		`"after":{"block":{"flavor":"com.superlogical.terminal.shell","label":"logs","options":{"cwd":"/w/x","command":["tail","-f","app.log"]}}}}}}}`
	if string(got) != want {
		t.Errorf("BuildNode JSON:\n got %s\nwant %s", got, want)
	}
}
