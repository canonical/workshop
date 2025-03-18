package progress

import (
	"fmt"
)

type rawDisplay struct {
	DefaultDisplay
	lastTask string
}

func (r *rawDisplay) Render(task string, b []byte, _, _ float64) {
	if r.lastTask != task {
		fmt.Fprintln(stdout, task)
		r.lastTask = task
	}
	fmt.Fprintln(stdout, b)
}

func (r *rawDisplay) Close() {
	r.DefaultDisplay.Close()
	fmt.Println()
}
