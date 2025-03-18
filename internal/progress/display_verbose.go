package progress

import (
	"bufio"
	"bytes"
	"fmt"
)

type VerboseDisplay struct {
	DefaultDisplay
	lines     []string
	maxLines  int
	viewLines int
}

func (w *VerboseDisplay) ClearData() {
	w.lines = make([]string, 0)
	w.viewLines = 0
}

func (w *VerboseDisplay) Render(task string, b []byte, current, total float64) {
	s := bufio.NewScanner(bytes.NewReader(b))
	for s.Scan() {
		if s.Text() == "" {
			continue
		}
		w.lines = append(w.lines, s.Text())
	}

	i := max(len(w.lines)-w.maxLines, 0)
	w.lines = w.lines[i:]

	w.DefaultDisplay.Render(task, nil, current, total)

	// Clear below spinner
	fmt.Printf("\n%s", clrEOS)

	fmt.Fprint(stdout, setBackground)

	// Print
	for _, line := range w.lines {
		if len(line) > w.width {
			line = line[:w.width]
		}
		// Erase line, then print (we don't need the erase, this however ensures
		// the background colour is applied to the entire width.
		fmt.Println(clrEOL + line)
	}

	fmt.Fprint(stdout, resetFormatting)

	w.viewLines = len(w.lines)
	// Reset cursor to top of 'VerboseDisplay'
	fmt.Printf(moveCursorUp, w.viewLines+1) // +1 for spinner
}

func (w *VerboseDisplay) Close() {
	w.DefaultDisplay.Close()
	// Reset cursor to bottom of 'VerboseDisplay', this preserves the last log
	// lines (if still present) and ensures future output exists in the correct
	// location
	if w.viewLines > 0 {
		fmt.Printf(moveCursorDown, w.viewLines)
	}
}
