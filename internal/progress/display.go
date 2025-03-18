package progress

import (
	"fmt"
	"os"
	"time"

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
	// Handle screen size changes
	width := termWidth()
	if d.width != width {
		d.width = width
		fmt.Fprint(stdout, clrEOS)
	}

	// Handle task changes
	if d.task.name != task {
		d.task.name = task
		d.task.startTime = time.Now().UTC()
	}

	// Trim task to match terminal width. This is an interactive shell, if a user
	// wants to see more output, they can make their shell larger, or use --raw.
	// In practice, this line rarely exceeds 40 chars which is comfortably
	// renderable on a 1/4 split vertical monitor.
	task = task[:min(len(task), d.width)]

	d.task.total = total
	d.task.current = current

	fmt.Fprint(stdout, clrEOL)

	// Task has no measurable progress, render a spinner
	if d.task.total == 1 || d.task.total == 0 {
		d.renderSpinner(task)
		return
	}

	d.renderProgress(task)
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
	// Taken in part from snapd -> ansimeter to ensure consistent output.
	// Modified to ensure that we have the requisite width for the current task
	// Widths (including single-space gap, used below):
	// 	time left: 6
	//		percent: 5
	// 						11 (cumulative total)
	// 			speed: 9
	//						20 (total)
	var percent, speed, timeleft string
	if d.width > len(msg)+6 {
		since := time.Now().UTC().Sub(d.task.startTime).Seconds()
		per := since / d.task.current
		left := (d.task.total - d.task.current) * per
		timeleft = " " + quantity.FormatDuration(left)
		if d.width > len(msg)+11 {
			percent = " " + d.percent()
			if d.width > len(msg)+20 {
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
	fmt.Fprint(stdout, setInverse, string(out[:i]), resetFormatting, string(out[i:]), "\r")
}

func (d *DefaultDisplay) renderSpinner(msg string) {
	remain := d.width - len(msg)
	if remain > 0 {
		fmt.Fprintf(stdout, "%s%*s\r", msg, remain, spinner[d.spin])
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
