// Package brand holds Akuaku's visual identity — the accent color, the tiki
// mask, and the AKUAKU wordmark — so the monitor and the CLI render the same
// mark from one source of truth. Every line is returned pre-styled; measure it
// with lipgloss.Width.
package brand

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Accent is Akuaku's signature aqua, used for the wordmark and section headings.
var Accent = lipgloss.Color("44")

// paint styles s with a 256-color code; colors collapse to plain text when the
// output is not a terminal, so callers can assert on content in tests.
func paint(code, s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(code)).Render(s)
}

// Mask draws the colorful tiki mask — feathers, eyes, mouth. The three lines are
// each five display columns wide so they align as a column beside any text.
func Mask() []string {
	return []string{
		" " + paint("99", `\`) + paint("220", "|") + paint("208", "/") + " ",
		paint("130", "(") + paint("220", "●") + " " + paint("220", "●") + paint("130", ")"),
		" " + paint("196", "╰—╯") + " ",
	}
}

// Wordmark spells AKUAKU in aqua block characters across two lines.
func Wordmark() []string {
	word := lipgloss.NewStyle().Bold(true).Foreground(Accent)
	return []string{
		word.Render("▄▀█ █▄▀ █ █ ▄▀█ █▄▀ █ █"),
		word.Render("█▀█ █▀▄ █▄█ █▀█ █▀▄ █▄█"),
	}
}

// Logo is the mask beside the wordmark: three lines, the wordmark on the first
// two and the mask's mouth on the third. This is the monitor's header mark.
func Logo() []string {
	return beside(Mask(), Wordmark())
}

// Header is the logo with a tagline placed on the mask's third line, so the CLI
// help opens with the full mark and a one-line description.
func Header(tagline string) []string {
	return beside(Mask(), append(Wordmark(), tagline))
}

// beside lays right after left, padding every left line to the widest so the
// right column aligns. right is never longer than left here, so a missing or
// blank right line simply contributes no trailing separator.
func beside(left, right []string) []string {
	leftW := 0
	for _, line := range left {
		if w := lipgloss.Width(line); w > leftW {
			leftW = w
		}
	}

	out := make([]string, len(left))
	for i, l := range left {
		out[i] = l + strings.Repeat(" ", leftW-lipgloss.Width(l))
		if i < len(right) && right[i] != "" {
			out[i] += "  " + right[i]
		}
	}
	return out
}
