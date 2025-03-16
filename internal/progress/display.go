package progress

import (
	"fmt"
	"unicode"

	"golang.org/x/term"
)

type DisplayMode int

const (
	DisplayModeDefault DisplayMode = iota
	DisplayModeVerbose
	DisplayModeRaw
)

var numDisplayLines = 7

type Display interface {
	ClearData()
	Render(task string, b []byte)
	Close()
}

func NewDisplay(mode DisplayMode) Display {
	// Hide cursor
	fmt.Print("\033[?25l")

	switch mode {
	case DisplayModeRaw:
		return &rawDisplay{}
	case DisplayModeVerbose:
		return &VerboseDisplay{maxLines: numDisplayLines, viewLines: -1}
	default:
		return &DefaultDisplay{}
	}
}

type DefaultDisplay struct {
	spin  int
	width int
}

func (d *DefaultDisplay) ClearData() {}

// Todo
//   - progress
func (d *DefaultDisplay) Render(task string, _ []byte) {
	d.width = termWidth()

	// Clear line
	fmt.Printf("\033[0K")

	// Print task, splits on nearest space where the line would exceed the term
	// width
	msg := []rune(task)
	var i int
	for len(msg) > d.width {
		for i = d.width; i >= 0; i-- {
			if unicode.IsSpace(msg[i]) {
				break
			}
		}
		if i < 1 {
			// didn't find anything; print the whole thing and try again
			fmt.Printf(string(msg[:d.width]))
			msg = msg[d.width:]
		} else {
			// found a space; print up to but not including it, and skip it
			fmt.Printf(string(msg[:i]))
			msg = msg[i+1:]
		}
	}

	remain := d.width - len(msg)
	if remain > 0 {
		fmt.Printf("%s%*s\r", string(msg), remain, spinner[d.spin])
		d.spin++
		if d.spin >= len(spinner) {
			d.spin = 0
		}
	}
}

func (d *DefaultDisplay) Close() {
	// Re-enable cursor
	fmt.Print("\033[?25h")
}

func termWidth() int {
	col, _, _ := term.GetSize(0)
	if col <= 0 {
		// default to 80.
		col = 80
	}
	return col
}
