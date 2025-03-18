package progress

import (
	"fmt"
	"os"
	"time"
	"unicode"

	"github.com/canonical/x-go/strutil/quantity"
	"golang.org/x/term"

	"github.com/canonical/workshop/internal/ptyutil"
)

type DisplayMode int

const (
	DisplayModeDefault DisplayMode = iota
	DisplayModeVerbose
	DisplayModeRaw
)

// Ansi escape sequences
const (
	// clear to end of line
	clrEOL = "\033[K"
	// clear to end of screen
	clrEOS = "\n\033[0J"
	// make cursor invisible
	cursorInvisible = "\033[?25l"
	// make cursor visible
	cursorVisible = "\033[?25h"
	// inverse text (white on black -> black on white)
	setInverse = "\033[7m"
	// set colour
	setBackground = "\033[48;5;238m"
	// reset formatting
	resetFormatting = "\033[0m"
	// move cursor %d lines up
	moveCursorUp = "\033[%dA"
	// move cursor %d lines down
	moveCursorDown = "\033[%dB"
)

var (
	numDisplayLines = 7
	stdout          = os.Stdout
	spinner         = []string{"/", "-", "\\", "|"}
)

type Display interface {
	Render(task string, log []byte, progress, total float64)
	Close()
}

func NewDisplay(mode DisplayMode) Display {

	switch mode {
	case DisplayModeRaw:
		return &rawDisplay{}
	case DisplayModeVerbose:
		fmt.Fprint(stdout, cursorInvisible)
		return &VerboseDisplay{maxLines: numDisplayLines, viewLines: -1}
	default:
		// Default to quiet if stdout is not a terminal
		if !ptyutil.IsTerminal(int(stdout.Fd())) {
			return &QuietDisplay{}
		}
		fmt.Fprint(stdout, cursorInvisible)
		return &DefaultDisplay{}
	}
}

type DefaultDisplay struct {
	spin  int
	width int
	task  taskInfo
}

type taskInfo struct {
	name      string
	startTime time.Time
	current   float64
	total     float64
}

func (d *DefaultDisplay) Render(task string, _ []byte, current, total float64) {
	d.width = termWidth()
	if d.task.name != task {
		// Task changed, reset
		d.task.name = task
		d.task.startTime = time.Now().UTC()
	}

	d.task.total = total
	d.task.current = current

	fmt.Fprint(stdout, clrEOL)

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
			fmt.Fprint(stdout, string(msg[:d.width]))
			msg = msg[d.width:]
		} else {
			// found a space; print up to but not including it, and skip it
			fmt.Fprint(stdout, string(msg[:i]))
			msg = msg[i+1:]
		}
	}

	// Task has no measurable progress, render a spinner
	if d.task.total == 1 || d.task.total == 0 {
		d.renderSpinner(string(msg))
		return
	}

	d.renderProgress(string(msg))
}

func (d *DefaultDisplay) Close() {
	// Re-enable cursor
	fmt.Fprint(stdout, cursorVisible)
}

func termWidth() int {
	width, _, _ := term.GetSize(0)
	if width <= 0 {
		// default to 80.
		width = 80
	}
	return width
}

func (d *DefaultDisplay) renderProgress(msg string) {
	// Taken from snapd -> ansimeter to ensure consistent output
	// time left: 5
	//    gutter: 1
	//     speed: 8
	//    gutter: 1
	//   percent: 4
	//    gutter: 1
	//          =====
	//           20
	// and we want to leave at least 10 for the label, so:
	//  * if      width <= 15, don't show any of this (progress bar is good enough)
	//  * if 15 < width <= 20, only show time left (time left + gutter = 6)
	//  * if 20 < width <= 29, also show percentage (percent + gutter = 5
	//  * if 29 < width      , also show speed (speed+gutter = 9)
	var percent, speed, timeleft string
	if d.width > 15 {
		since := time.Now().UTC().Sub(d.task.startTime).Seconds()
		per := since / d.task.current
		left := (d.task.total - d.task.current) * per
		timeleft = " " + quantity.FormatDuration(left)
		if d.width > 20 {
			percent = " " + d.percent()
			if d.width > 29 {
				speed = " " + quantity.FormatBPS(d.task.current, since, -1)
			}
		}
	}

	out := make([]rune, 0, d.width)
	out = append(out, norm(d.width-len(percent)-len(speed)-len(timeleft), []rune(msg))...)
	out = append(out, []rune(percent)...)
	out = append(out, []rune(speed)...)
	out = append(out, []rune(timeleft)...)
	i := int(d.task.current * float64(d.width) / d.task.total)
	fmt.Fprint(stdout, "\r", setInverse, string(out[:i]), resetFormatting, string(out[i:]))
}

func (d *DefaultDisplay) renderSpinner(msg string) {
	remain := d.width - len(msg)
	if remain > 0 {
		fmt.Printf("%s%*s\r", msg, remain, spinner[d.spin])
		d.spin++
		if d.spin >= len(spinner) {
			d.spin = 0
		}
	}
}

func (d *DefaultDisplay) percent() string {
	if d.task.total == 0. {
		return "---%"
	}
	q := d.task.current * 100 / d.task.total
	if q > 999.4 || q < 0. {
		return "???%"
	}
	return fmt.Sprintf("%3.0f%%", q)
}

// QuietDisplay is a display that shows nothing.
type QuietDisplay struct {
	DefaultDisplay
}

func (q *QuietDisplay) Render(_ string, _ []byte, _, _ float64) {}

func (q *QuietDisplay) Close() {}

func norm(col int, msg []rune) []rune {
	if col <= 0 {
		return []rune{}
	}
	out := make([]rune, col)
	copy(out, msg)
	d := col - len(msg)
	if d < 0 {
		out[col-1] = '…'
	} else {
		for i := len(msg); i < col; i++ {
			out[i] = ' '
		}
	}
	return out
}
