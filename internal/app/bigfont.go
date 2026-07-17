package app

import "strings"

// Digit glyphs use the same 5x5 dot-matrix bitmap font as
// https://github.com/sectore/timr-tui (src/widgets/clock_elements.rs):
// 1 = filled cell, 0 = empty, read left-to-right, top-to-bottom.
const (
	glyphSize   = 5
	colonWidth  = 4
	glyphSymbol = "█"
)

var digitPatterns = map[rune][glyphSize * glyphSize]int{
	'0': {
		1, 1, 1, 1, 1,
		1, 1, 0, 1, 1,
		1, 1, 0, 1, 1,
		1, 1, 0, 1, 1,
		1, 1, 1, 1, 1,
	},
	'1': {
		0, 0, 0, 1, 1,
		0, 0, 0, 1, 1,
		0, 0, 0, 1, 1,
		0, 0, 0, 1, 1,
		0, 0, 0, 1, 1,
	},
	'2': {
		1, 1, 1, 1, 1,
		0, 0, 0, 1, 1,
		1, 1, 1, 1, 1,
		1, 1, 0, 0, 0,
		1, 1, 1, 1, 1,
	},
	'3': {
		1, 1, 1, 1, 1,
		0, 0, 0, 1, 1,
		1, 1, 1, 1, 1,
		0, 0, 0, 1, 1,
		1, 1, 1, 1, 1,
	},
	'4': {
		1, 1, 0, 1, 1,
		1, 1, 0, 1, 1,
		1, 1, 1, 1, 1,
		0, 0, 0, 1, 1,
		0, 0, 0, 1, 1,
	},
	'5': {
		1, 1, 1, 1, 1,
		1, 1, 0, 0, 0,
		1, 1, 1, 1, 1,
		0, 0, 0, 1, 1,
		1, 1, 1, 1, 1,
	},
	'6': {
		1, 1, 1, 1, 1,
		1, 1, 0, 0, 0,
		1, 1, 1, 1, 1,
		1, 1, 0, 1, 1,
		1, 1, 1, 1, 1,
	},
	'7': {
		1, 1, 1, 1, 1,
		0, 0, 0, 1, 1,
		0, 0, 0, 1, 1,
		0, 0, 0, 1, 1,
		0, 0, 0, 1, 1,
	},
	'8': {
		1, 1, 1, 1, 1,
		1, 1, 0, 1, 1,
		1, 1, 1, 1, 1,
		1, 1, 0, 1, 1,
		1, 1, 1, 1, 1,
	},
	'9': {
		1, 1, 1, 1, 1,
		1, 1, 0, 1, 1,
		1, 1, 1, 1, 1,
		0, 0, 0, 1, 1,
		1, 1, 1, 1, 1,
	},
}

// digitGlyph returns the glyphSize rows of a single digit, glyphSize columns wide.
func digitGlyph(d rune) []string {
	pattern, ok := digitPatterns[d]
	if !ok {
		return blankGlyph(glyphSize)
	}
	rows := make([]string, glyphSize)
	for y := 0; y < glyphSize; y++ {
		var row strings.Builder
		for x := 0; x < glyphSize; x++ {
			if pattern[y*glyphSize+x] == 1 {
				row.WriteString(glyphSymbol)
			} else {
				row.WriteString(" ")
			}
		}
		rows[y] = row.String()
	}
	return rows
}

// colonGlyph mirrors timr-tui's Colon widget: two filled cells at columns
// 1-2 on rows 1 and 3 of a colonWidth x glyphSize area.
func colonGlyph() []string {
	rows := blankGlyph(colonWidth)
	filled := " " + glyphSymbol + glyphSymbol + " "
	rows[1] = filled
	rows[3] = filled
	return rows
}

func blankGlyph(width int) []string {
	rows := make([]string, glyphSize)
	for i := range rows {
		rows[i] = strings.Repeat(" ", width)
	}
	return rows
}

// BigTime renders a time string like "25:00" or "01:25:00" as multi-line
// dot-matrix block digits, joined with a one-space gap between glyphs.
func BigTime(s string) []string {
	out := make([]string, glyphSize)
	first := true
	for _, ch := range s {
		var g []string
		if ch == ':' {
			g = colonGlyph()
		} else {
			g = digitGlyph(ch)
		}
		for i := 0; i < glyphSize; i++ {
			sep := " "
			if first {
				sep = ""
			}
			out[i] += sep + g[i]
		}
		first = false
	}
	return out
}
