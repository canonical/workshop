package progress

import (
	"fmt"
	"os"
	"time"

	"github.com/canonical/x-go/strutil/quantity"
	"golang.org/x/term"

	"github.com/canonical/workshop/client"
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
	clrEOS = "\033[0J"
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
	// Buffer returns the underlying buffer for logs
	Buffer() *[]byte
	// Render should be called in a loop. It takes a task and renders an
	// appropriate output
	Render(task client.Task)
	// Error renders the message. It should be used when it is not suitable or
	// possible to provide a task. (e.g. a backend error)
	Errorf(message string)
	// Flush should be called after each render loop, this achieves two things:
	//  1. If no tasks are in the doing state, the spinner will still work
	//  2. Any logs that were not displayed whilst the task was in 'doing' will
	//     have a chance to be rendered
	Flush()
	// Close should be called when the display is no longer needed. This ensures
	// any formatting is returned to the default
	Close()
}

func NewDisplay(mode DisplayMode) Display {
	d := DefaultDisplay{lastTask: &client.Task{}, buffer: new([]byte)}
	switch mode {
	case DisplayModeRaw:
		return &RawDisplay{DefaultDisplay: d}
	case DisplayModeVerbose:
		fmt.Fprint(stdout, cursorInvisible)
		return &VerboseDisplay{DefaultDisplay: d, maxLines: numDisplayLines, viewLines: -1}
	default:
		// Default to quiet if stdout is not a terminal
		if !ptyutil.IsTerminal(int(stdout.Fd())) {
			return &QuietDisplay{}
		}
		fmt.Fprint(stdout, cursorInvisible)
		return &d
	}
}

type DefaultDisplay struct {
	spin     int
	haveSpun bool
	width    int
	buffer   *[]byte
	lastTask *client.Task
}

func (d *DefaultDisplay) Buffer() *[]byte {
	return d.buffer
}

func (d *DefaultDisplay) Render(task client.Task) {
	d.lastTask = &task

	// Handle screen size changes
	width := termWidth()
	if d.width != width {
		d.width = width
		fmt.Fprint(stdout, clrEOS)
	}

	// Trim task summary to match terminal width. This is an interactive shell,
	// if a user wants to see more output, they can make their shell larger, or
	// use --raw. In practice, this line rarely exceeds 40 characters which is
	// comfortably renderable in nearly all circumstances.
	task.Summary = task.Summary[:min(len(task.Summary), d.width)]

	fmt.Fprint(stdout, clrEOL)

	// Task has no measurable progress, render a spinner
	if task.Progress.Total == 1 || task.Progress.Total == 0 {
		d.renderSpinner()
		return
	}

	d.renderProgress()
}

func (d *DefaultDisplay) Errorf(msg string) {
	d.renderSpinner()
}

func (d *DefaultDisplay) Flush() {
	if !d.haveSpun {
		d.renderSpinner()
	}
	*d.buffer = []byte{}
	d.haveSpun = false
}

func (d *DefaultDisplay) Close() {
	fmt.Fprint(stdout, clrEOL)
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

func (d *DefaultDisplay) renderProgress() {
	// Taken in part from snapd -> ansimeter to ensure consistent output.
	// Modified to ensure that we have the requisite width for the current task.
	//
	// Reference widths (including single-space gap):
	// 	time left: 6
	//		percent: 5
	// 						11 (cumulative total)
	// 			speed: 9
	//						20 (total)
	var percent, speed, timeleft string
	if d.width > len(d.lastTask.Summary)+6 {
		since := time.Now().UTC().Sub(d.lastTask.SpawnTime).Seconds()
		per := since / float64(d.lastTask.Progress.Done)
		left := (float64(d.lastTask.Progress.Total) - float64(d.lastTask.Progress.Done)) * per
		timeleft = " " + quantity.FormatDuration(left)
		if d.width > len(d.lastTask.Summary)+11 {
			percent = " " + d.percent(d.lastTask)
			if d.width > len(d.lastTask.Summary)+20 {
				speed = " " + quantity.FormatBPS(float64(d.lastTask.Progress.Done), since, -1)
			}
		}
	}

	out := make([]rune, 0, d.width)
	out = append(out, norm(d.width-len(percent)-len(speed)-len(timeleft), []rune(d.lastTask.Summary))...)
	out = append(out, []rune(percent)...)
	out = append(out, []rune(speed)...)
	out = append(out, []rune(timeleft)...)
	i := int(float64(d.lastTask.Progress.Done) * float64(d.width) / float64(d.lastTask.Progress.Total))
	fmt.Fprint(stdout, setInverse, string(out[:i]), resetFormatting, string(out[i:]), "\r")
}

func (d *DefaultDisplay) renderSpinner() {
	remain := d.width - len(d.lastTask.Summary)
	if remain > 0 {
		fmt.Fprintf(stdout, "%s%*s\r", d.lastTask.Summary, remain, spinner[d.spin])
		d.spin++
		if d.spin >= len(spinner) {
			d.spin = 0
		}
		d.haveSpun = true
		return
	}
	// No room for the spinner
	fmt.Fprintf(stdout, "%s", d.lastTask.Summary)
}

func (d *DefaultDisplay) percent(t *client.Task) string {
	if float64(t.Progress.Done) == 0. {
		return "---%"
	}
	q := float64(t.Progress.Done) * 100 / float64(t.Progress.Total)
	if q > 999.4 || q < 0. {
		return "???%"
	}
	return fmt.Sprintf("%3.0f%%", q)
}

// QuietDisplay is a display that shows nothing.
type QuietDisplay struct {
	DefaultDisplay
}

func (q *QuietDisplay) Render(_ client.Task) {}

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
