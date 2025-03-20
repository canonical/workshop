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
	if r.taskLog.TaskID != task.ID {
		r.printAndClear()
	}

	if r.lastTask.ID != task.ID {
		fmt.Fprintf(stdout, "%s%s\n", "TASK: ", task.Summary)
		r.lastTask.ID = task.ID
	}

	r.printAndClear()
}

func (r *RawDisplay) Flush() {
	r.printAndClear()
}

func (r *RawDisplay) Close() {
	r.DefaultDisplay.Close()
	fmt.Println()
}

func (r *RawDisplay) printAndClear() {
	// Compress output, ignore empty and newlines
	if len(r.taskLog.Log) > 0 && string(r.taskLog.Log) != "\n" {
		fmt.Fprintln(stdout, strings.TrimSuffix(string(r.taskLog.Log), "\n"))
	}
	r.taskLog.Log = []byte{}
}
