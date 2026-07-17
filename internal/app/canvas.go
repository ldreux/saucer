package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// cellStyle holds only the visual attributes a cell needs. It's a plain
// comparable struct (lipgloss.Color is just a string type) so Canvas.Render
// can group consecutive same-styled cells into a single ANSI run.
type cellStyle struct {
	fg   lipgloss.Color
	bold bool
}

type cell struct {
	ch    rune
	style cellStyle
	set   bool
}

// Canvas is a per-cell buffer that lets a background layer (weather) and
// foreground layer (time/status/footer text) be composited: foreground
// writes skip literal spaces, so background cells show through gaps.
type Canvas struct {
	width, height int
	cells         [][]cell
}

func NewCanvas(width, height int) *Canvas {
	c := &Canvas{}
	c.Reset(width, height)
	return c
}

// Reset clears the canvas for a new frame. If the dimensions match the
// existing buffer it clears cells in place (no allocation); it only
// reallocates the backing 2D slice when the size actually changed (e.g. a
// terminal resize), so repeated per-frame calls at a stable size are cheap.
func (c *Canvas) Reset(width, height int) {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	if width == c.width && height == c.height {
		for _, row := range c.cells {
			for i := range row {
				row[i] = cell{}
			}
		}
		return
	}
	c.width, c.height = width, height
	c.cells = make([][]cell, height)
	for i := range c.cells {
		c.cells[i] = make([]cell, width)
	}
}

func (c *Canvas) Set(row, col int, ch rune, style cellStyle) {
	if row < 0 || row >= c.height || col < 0 || col >= c.width {
		return
	}
	c.cells[row][col] = cell{ch: ch, style: style, set: true}
}

// WriteTextSkipSpaces writes text left-to-right starting at (row, col).
// Literal space runes are skipped rather than overwriting the cell, so
// anything already drawn there (e.g. a weather drop) remains visible.
func (c *Canvas) WriteTextSkipSpaces(row, col int, text string, style cellStyle) {
	i := 0
	for _, r := range text {
		if r != ' ' {
			c.Set(row, col+i, r, style)
		}
		i++
	}
}

// styleANSICache memoizes the ANSI escape prefix/suffix lipgloss produces
// for a given cellStyle, keyed by the style itself. It's populated lazily by
// actually asking lipgloss to render a sentinel through the style once (so
// the terminal's detected color profile / fallback behavior is respected
// exactly as before), then reused for every run sharing that style instead
// of re-invoking lipgloss.Style.Render()'s full pipeline per run.
var styleANSICache = map[cellStyle][2]string{}

const ansiSentinel = "\x01SENTINEL\x01"

func ansiFor(style cellStyle) (prefix, suffix string) {
	if cached, ok := styleANSICache[style]; ok {
		return cached[0], cached[1]
	}
	rendered := lipgloss.NewStyle().Foreground(style.fg).Bold(style.bold).Render(ansiSentinel)
	idx := strings.Index(rendered, ansiSentinel)
	if idx < 0 {
		styleANSICache[style] = [2]string{"", ""}
		return "", ""
	}
	prefix = rendered[:idx]
	suffix = rendered[idx+len(ansiSentinel):]
	styleANSICache[style] = [2]string{prefix, suffix}
	return prefix, suffix
}

// Render walks each row and merges consecutive cells sharing the same style
// into single runs, wrapping each in its cached ANSI prefix/suffix instead
// of a full lipgloss.Style.Render() call, keeping per-frame cost independent
// of how many cells share a style.
func (c *Canvas) Render() string {
	var out strings.Builder
	for row := 0; row < c.height; row++ {
		var runText strings.Builder
		var runStyle cellStyle
		runActive := false

		flush := func() {
			if runText.Len() == 0 {
				return
			}
			if runActive {
				prefix, suffix := ansiFor(runStyle)
				out.WriteString(prefix)
				out.WriteString(runText.String())
				out.WriteString(suffix)
			} else {
				out.WriteString(runText.String())
			}
			runText.Reset()
		}

		for col := 0; col < c.width; col++ {
			cl := c.cells[row][col]
			ch := cl.ch
			if !cl.set {
				ch = ' '
			}
			active := cl.set
			if runText.Len() > 0 && (active != runActive || cl.style != runStyle) {
				flush()
			}
			runActive = active
			runStyle = cl.style
			runText.WriteRune(ch)
		}
		flush()

		if row < c.height-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
