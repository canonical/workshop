package progress

import (
	"bufio"
	"bytes"
	"fmt"
)

type rawDisplay struct {
	DefaultDisplay
	lastTask string
}

func (r *rawDisplay) Render(task string, b []byte, _, _ float64) {
	var lines []string
	s := bufio.NewScanner(bytes.NewReader(b))
	for s.Scan() {
		if s.Text() == "" {
			continue
		}
		lines = append(lines, s.Text())
	}

	if r.lastTask != task {
		fmt.Println(task)
		r.lastTask = task
	}
	for _, line := range lines {
		fmt.Println(line)
	}
}

func (r *rawDisplay) Close() {
	r.DefaultDisplay.Close()
	fmt.Println()
}
