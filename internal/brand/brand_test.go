package brand_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/akuaku-ai/akuaku/internal/brand"
)

// The mask is three lines, each the same display width, so it aligns as a column
// beside any text placed to its right.
func TestMaskIsThreeAlignedLines(t *testing.T) {
	mask := brand.Mask()
	if len(mask) != 3 {
		t.Fatalf("mask lines = %d, want 3", len(mask))
	}
	want := lipgloss.Width(mask[0])
	if want == 0 {
		t.Fatal("mask line 0 is empty")
	}
	for i, line := range mask {
		if got := lipgloss.Width(line); got != want {
			t.Errorf("mask line %d width = %d, want %d", i, got, want)
		}
	}
}

// The wordmark spells AKUAKU across two equal-width block-character lines.
func TestWordmarkIsTwoAlignedLines(t *testing.T) {
	word := brand.Wordmark()
	if len(word) != 2 {
		t.Fatalf("wordmark lines = %d, want 2", len(word))
	}
	if lipgloss.Width(word[0]) != lipgloss.Width(word[1]) {
		t.Errorf("wordmark line widths differ: %d vs %d", lipgloss.Width(word[0]), lipgloss.Width(word[1]))
	}
	if lipgloss.Width(word[0]) == 0 {
		t.Error("wordmark is empty")
	}
}

// Logo places the wordmark beside the mask: three lines, the wordmark on the
// first two, the mask spanning all three.
func TestLogoPlacesWordmarkBesideMask(t *testing.T) {
	logo := brand.Logo()
	if len(logo) != 3 {
		t.Fatalf("logo lines = %d, want 3", len(logo))
	}
	if !strings.Contains(logo[0], "█") {
		t.Errorf("logo line 0 = %q, want the wordmark block characters", logo[0])
	}
	// Every logo line is at least as wide as the mask, since the mask leads each.
	maskW := lipgloss.Width(brand.Mask()[0])
	for i, line := range logo {
		if lipgloss.Width(line) < maskW {
			t.Errorf("logo line %d width = %d, narrower than the mask (%d)", i, lipgloss.Width(line), maskW)
		}
	}
}

// Header is the logo with a tagline on the mask's third line, for the CLI help.
func TestHeaderAddsTaglineOnThirdLine(t *testing.T) {
	header := brand.Header("monitor & launch")
	if len(header) != 3 {
		t.Fatalf("header lines = %d, want 3", len(header))
	}
	if !strings.Contains(header[0], "█") {
		t.Errorf("header line 0 = %q, want the wordmark", header[0])
	}
	if !strings.Contains(header[2], "monitor & launch") {
		t.Errorf("header line 2 = %q, want the tagline", header[2])
	}
}
