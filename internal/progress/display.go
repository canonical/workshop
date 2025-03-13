package progress

import (
	"fmt"

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
		width, _, _ := term.GetSize(0)
		return &VerboseDisplay{DefaultDisplay: DefaultDisplay{width: width}, maxLines: numDisplayLines, viewLines: -1}
	default:
		width, _, _ := term.GetSize(0)
		return &DefaultDisplay{width: width}
	}
}

type DefaultDisplay struct {
	spin  int
	width int
}

func (d *DefaultDisplay) ClearData() {}

// Todo
//   - fancy line wrapping
//   - progress
func (d *DefaultDisplay) Render(task string, _ []byte) {
	// Print summary and spin
	remain := d.width - len(task)
	fmt.Printf("\033[0K%s%*s\r", task, remain, spinner[d.spin])
	d.spin++
	if d.spin >= len(spinner) {
		d.spin = 0
	}
}

func (d *DefaultDisplay) Close() {
	// Re-enable cursor
	fmt.Print("\033[?25h")
}
