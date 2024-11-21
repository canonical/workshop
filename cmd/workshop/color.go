// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2018 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package main

import (
	"os"
	"strings"

	"github.com/canonical/x-go/i18n"
	"golang.org/x/term"
)

var isStdinTTY = term.IsTerminal(0)

type mixinDescs map[string]string

func (mxd mixinDescs) also(m map[string]string) mixinDescs {
	n := make(map[string]string, len(mxd)+len(m))
	for k, v := range mxd {
		n[k] = v
	}
	for k, v := range m {
		n[k] = v
	}
	return n
}

type unicodeMixin struct {
	Unicode string
}

func (ux unicodeMixin) addUnicodeChars(esc *escapes) {
	if canUnicode(ux.Unicode) {
		esc.dash = "–" // that's an en dash (so yaml is happy)
		esc.uparrow = "↑"
		esc.tick = "✓"
		esc.star = "✪"
	} else {
		esc.dash = "--" // two dashes keeps yaml happy also
		esc.uparrow = "^"
		esc.tick = "**"
		esc.star = "*"
	}
}

func (ux unicodeMixin) getEscapes() *escapes {
	esc := &escapes{}
	ux.addUnicodeChars(esc)
	return esc
}

type colorMixin struct {
	Color string
	unicodeMixin
}

func (mx colorMixin) getEscapes() *escapes {
	esc := colorTable(mx.Color)
	mx.addUnicodeChars(&esc)
	return &esc
}

func canUnicode(mode string) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	}
	if !isStdoutTTY {
		return false
	}
	var lang string
	for _, k := range []string{"LC_MESSAGES", "LC_ALL", "LANG"} {
		lang = os.Getenv(k)
		if lang != "" {
			break
		}
	}
	if lang == "" {
		return false
	}
	lang = strings.ToUpper(lang)
	return strings.Contains(lang, "UTF-8") || strings.Contains(lang, "UTF8")
}

var isStdoutTTY = term.IsTerminal(1)

func colorTable(mode string) escapes {
	switch mode {
	case "always":
		return color
	case "never":
		return noesc
	}
	if !isStdoutTTY {
		return noesc
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		// from http://no-color.org/:
		//   command-line software which outputs text with ANSI color added should
		//   check for the presence of a NO_COLOR environment variable that, when
		//   present (regardless of its value), prevents the addition of ANSI color.
		return mono // bold & dim is still ok
	}
	if term := os.Getenv("TERM"); term == "xterm-mono" || term == "linux-m" {
		// these are often used to flag "I don't want to see color" more than "I can't do color"
		// (if you can't *do* color, `color` and `mono` should produce the same results)
		return mono
	}
	return color
}

var colorDescs = mixinDescs{
	// TRANSLATORS: This should not start with a lowercase letter.
	"color":   i18n.G("Use a little bit of color to highlight some things."),
	"unicode": unicodeDescs["unicode"],
}

var unicodeDescs = mixinDescs{
	// TRANSLATORS: This should not start with a lowercase letter.
	"unicode": i18n.G("Use a little bit of Unicode to improve legibility."),
}

type escapes struct {
	green        string
	brightYellow string
	bold         string
	end          string

	tick, dash, uparrow, star string
}

var (
	color = escapes{
		green:        "\033[32m",
		brightYellow: "\033[93m",
		bold:         "\033[1m",
		end:          "\033[0m",
	}

	mono = escapes{
		green:        "\033[1m", // bold
		brightYellow: "\033[2m", // dim
		bold:         "\033[1m",
		end:          "\033[0m",
	}

	noesc = escapes{}
)

// fillerPublisher is used to add an no-op escape sequence to a header in a
// tabwriter table, so that things line up.
func fillerPublisher(esc *escapes) string {
	return esc.green + esc.end
}
