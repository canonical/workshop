// Copyright (c) 2014-2020 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package progress

import (
	"fmt"
	"io"
	"os"
	"time"
	"unicode"

	"github.com/canonical/x-go/strutil/quantity"
	"golang.org/x/term"
)

var stdout io.Writer = os.Stdout

type notifyer interface {
	Notify(msg string)
}

func NewANSIMeter(nt NotifierType) *ANSIMeter {
	switch nt {
	case NotifierQuiet:
		return &ANSIMeter{
			notifier: &nilNotifier{},
		}
	case NotifierRaw:
		return &ANSIMeter{
			notifier: &rawPrintNotifier{},
		}
	case NotifierLimitedWindow:
		return &ANSIMeter{
			notifier: newLimitedWindowNotifier(defaultNotificationLines),
		}
	default:
		panic("unknown notifier type for ANSIMeter")
	}
}

// ANSIMeter is a progress.Meter that uses ANSI escape codes to make
// better use of the available horizontal space.
type ANSIMeter struct {
	label    []rune
	total    float64
	written  float64
	spin     int
	t0       time.Time
	notifier notifyer
}

// these are the bits of the ANSI escapes (beyond \r) that we use
// (names of the terminfo capabilities, see terminfo(5))
var (
	// clear to end of line
	clrEOL = "\033[K"
	clrEOS = "\033[0J"
	// make cursor invisible
	cursorInvisible = "\033[?25l"
	// make cursor visible
	cursorVisible = "\033[?25h"
	// turn on reverse video
	enterReverseMode = "\033[7m"
	// go back to normal video
	exitAttributeMode = "\033[0m"
	// move cursor %d lines up
	moveCursorUp = "\033[%dA"
)

const defaultNotificationLines = 5

type nilNotifier struct{}

func (*nilNotifier) Notify(string) {}

// limitedWindowNotifier manages a scrolling fixed-size window for notifications.
type limitedWindowNotifier struct {
	history  []string
	numLines int
}

func newLimitedWindowNotifier(numLines int) *limitedWindowNotifier {
	if numLines <= 0 {
		numLines = defaultNotificationLines
	}
	return &limitedWindowNotifier{
		history:  make([]string, 0, numLines),
		numLines: numLines,
	}
}

// Notify adds a message and displays the updated window to stdout.
// New messages effectively appear at the bottom of the window, and old messages scroll off the top.
func (lwn *limitedWindowNotifier) Notify(msg string) {
	lwn.history = append(lwn.history, msg)

	if len(lwn.history) > lwn.numLines {
		lwn.history = lwn.history[len(lwn.history)-lwn.numLines:]
	}

	linesToPrint := len(lwn.history)

	if linesToPrint > 0 {
		fmt.Fprint(stdout, "\n")
		fmt.Fprint(stdout, clrEOS)

		for i := 0; i < linesToPrint; i++ {
			fmt.Fprint(stdout, "\r", exitAttributeMode, clrEOL)
			fmt.Fprint(stdout, lwn.history[i])

			if i < linesToPrint-1 { // For all but the last line printed
				fmt.Fprint(stdout, "\n")
			}
		}
		fmt.Fprintf(stdout, moveCursorUp, linesToPrint)
		fmt.Fprint(stdout, "\r")
	}
}

type rawPrintNotifier struct {
}

func (*rawPrintNotifier) Notify(msgstr string) {
	col := termWidth()
	// Clear the current line, reset attributes, and move cursor to the beginning.
	fmt.Fprint(stdout, "\r", exitAttributeMode, clrEOL)

	msg := []rune(msgstr)
	var breakPoint int
	// Word wrap the message if it's longer than the terminal width.
	for len(msg) > col {
		breakPoint = col // Default break point if no space is found within the first `col` characters.
		// Find the last space within the column width to break the line.
		// Search from right to left within the available width.
		for searchIdx := col - 1; searchIdx >= 0; searchIdx-- {
			if unicode.IsSpace(msg[searchIdx]) {
				breakPoint = searchIdx
				break // Found a suitable space.
			}
		}

		if breakPoint == col { // No space found (or space is beyond `col`), hard break at `col`.
			fmt.Fprintln(stdout, string(msg[:col]))
			msg = msg[col:]
		} else { // Space found at breakPoint (0 <= breakPoint < col).
			fmt.Fprintln(stdout, string(msg[:breakPoint])) // Print up to the space.
			msg = msg[breakPoint+1:]                       // Continue with the rest, skipping the space.
		}
	}
	fmt.Fprintln(stdout, string(msg)) // Print the remaining part of the message.
}

var termWidth = func() int {
	col, _, _ := term.GetSize(0)
	if col <= 0 {
		// give up
		col = 80
	}
	return col
}

func (p *ANSIMeter) Start(label string, total float64) {
	p.label = []rune(label)
	p.total = total
	p.t0 = time.Now().UTC()
	fmt.Fprint(stdout, cursorInvisible)
}

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

func (p *ANSIMeter) SetTotal(total float64) {
	p.total = total
}

func (p *ANSIMeter) percent() string {
	if p.total == 0. {
		return "---%"
	}
	q := p.written * 100 / p.total
	if q > 999.4 || q < 0. {
		return "???%"
	}
	return fmt.Sprintf("%3.0f%%", q)
}

func (p *ANSIMeter) Set(current float64) {
	if current < 0 {
		current = 0
	}
	if current > p.total {
		current = p.total
	}

	p.written = current
	col := termWidth()
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
	if col > 15 {
		since := time.Now().UTC().Sub(p.t0).Seconds()
		per := since / p.written
		left := (p.total - p.written) * per
		timeleft = " " + quantity.FormatDuration(left)
		if col > 20 {
			percent = " " + p.percent()
			if col > 29 {
				speed = " " + quantity.FormatBPS(p.written, since, -1)
			}
		}
	}

	msg := make([]rune, 0, col)
	msg = append(msg, norm(col-len(percent)-len(speed)-len(timeleft), p.label)...)
	msg = append(msg, []rune(percent)...)
	msg = append(msg, []rune(speed)...)
	msg = append(msg, []rune(timeleft)...)
	i := int(current * float64(col) / p.total)
	fmt.Fprint(stdout, "\r", enterReverseMode, string(msg[:i]), exitAttributeMode, string(msg[i:]))
}

var spinner = []string{"/", "-", "\\", "|"}

func (p *ANSIMeter) Spin(msgstr string) {
	msg := []rune(msgstr)
	col := termWidth()
	if col-2 >= len(msg) {
		fmt.Fprint(stdout, "\r", string(norm(col-2, msg)), " ", spinner[p.spin])
		p.spin++
		if p.spin >= len(spinner) {
			p.spin = 0
		}
	} else {
		fmt.Fprint(stdout, "\r", string(norm(col, msg)))
	}
}

func (*ANSIMeter) Finished() {
	fmt.Fprint(stdout, "\r", exitAttributeMode, cursorVisible, clrEOL)
}

func (p *ANSIMeter) Notify(msgstr string) {
	p.notifier.Notify(msgstr)
}

func (p *ANSIMeter) Write(bs []byte) (n int, err error) {
	n = len(bs)
	p.Set(p.written + float64(n))

	return
}
