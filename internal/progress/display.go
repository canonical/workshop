package progress

import (
	"fmt"
	"time"
	"unicode"

	"github.com/canonical/x-go/strutil/quantity"
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
	Render(task string, log []byte, progress, total float64)
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

	// Task has no measurable progress, render a spinner
	if d.task.total == 1 || d.task.total == 0 {
		d.renderSpinner(string(msg))
		return
	}

	d.renderProgress(string(msg))
}

func (d *DefaultDisplay) Close() {
	// Re-enable cursor
	fmt.Print("\033[?25h")
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
	fmt.Fprint(stdout, "\r", enterReverseMode, string(out[:i]), exitAttributeMode, string(out[i:]))
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
