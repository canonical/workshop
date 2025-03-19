package progress

import (
	"fmt"
	"strings"

	"github.com/canonical/workshop/client"
)

type RawDisplay struct {
	DefaultDisplay
}

// Renders progress in the following format:
// TASK: <task summary>
// <hook stdout (where applicable)>
func (r *RawDisplay) Render(task client.Task) {
	if r.lastTask.ID != task.ID {
		fmt.Fprintf(stdout, "%s%s\n", "TASK: ", task.Summary)
		r.lastTask.ID = task.ID
	}
	// Compress output, ignore empty and newlines
	if len(*r.buffer) > 0 && string(*r.buffer) != "\n" {
		fmt.Fprintln(stdout, strings.TrimSuffix(string(*r.buffer), "\n"))
	}
	*r.buffer = []byte{}
}

func (r *RawDisplay) Flush() {
	if len(*r.buffer) > 0 {
		fmt.Fprintln(stdout, strings.TrimSuffix(string(*r.buffer), "\n"))
	}
	*r.buffer = []byte{}
}

func (r *RawDisplay) Close() {
	r.DefaultDisplay.Close()
	fmt.Println()
}
