package progress

import (
	"bufio"
	"bytes"
	"fmt"

	"github.com/canonical/workshop/client"
)

type VerboseDisplay struct {
	DefaultDisplay
	lines      []string
	maxLines   int
	viewLines  int
	lastTaskID string
}

func (v *VerboseDisplay) Render(task client.Task) {
	if task.ID != v.lastTaskID {
		v.lines = make([]string, 0)
		v.viewLines = 0
		v.lastTaskID = task.ID
	}

	v.appendLines()

	v.DefaultDisplay.Render(task)

	v.renderLines()
	v.taskLog.Log = []byte{}
}

func (v *VerboseDisplay) Flush() {
	if !v.haveSpun {
		v.renderSpinner()
	}
	v.haveSpun = false
	v.appendLines()
	v.renderLines()
	v.taskLog.Log = []byte{}
}

func (v *VerboseDisplay) Close() {
	fmt.Fprint(stdout, cursorVisible)
	// Reset cursor to bottom of 'VerboseDisplay', this preserves the last log
	// lines (if still present) and ensures future output exists at the correct
	// location
	if v.viewLines > 1 {
		fmt.Fprintf(stdout, moveCursorDown, v.viewLines)
		return
	}
	fmt.Fprint(stdout, clrEOL)
}

func (v *VerboseDisplay) appendLines() {
	s := bufio.NewScanner(bytes.NewReader(v.taskLog.Log))
	for s.Scan() {
		if s.Text() == "" {
			continue
		}
		v.lines = append(v.lines, s.Text())
	}

	i := max(len(v.lines)-v.maxLines, 0)
	v.lines = v.lines[i:]
}

func (v *VerboseDisplay) renderLines() {
	// Clear below spinner
	fmt.Printf("\n%s", clrEOS)

	fmt.Fprint(stdout, setBackground)

	// Print
	for _, line := range v.lines {
		if len(line) > v.width {
			line = line[:v.width]
		}
		// Erase line, then print (we don't need the erase, this however ensures
		// the background colour is applied to the entire width.
		fmt.Println(clrEOL + line)
	}

	fmt.Fprint(stdout, resetFormatting)

	v.viewLines = len(v.lines) + 1 // +1 for spinner
	// Reset cursor to top of 'VerboseDisplay'
	fmt.Printf(moveCursorUp, v.viewLines)
}
