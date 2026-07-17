package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const footerContentWidth = 64

// segment is a run of text with a single style; a line is built from one or
// more segments so mixed-color rows (dots, footer) can be represented, then
// written into the Canvas so the weather background shows through spaces.
type segment struct {
	text  string
	style cellStyle
}

func segWidth(segs []segment) int {
	w := 0
	for _, s := range segs {
		w += lipgloss.Width(s.text)
	}
	return w
}

// centerCol matches lipgloss.PlaceHorizontal's behavior for oversized
// content: left-align (col 0) instead of clipping the left edge when the
// content is wider than the available space.
func centerCol(width, contentWidth int) int {
	col := (width - contentWidth) / 2
	if col < 0 {
		return 0
	}
	return col
}

func writeSegments(c *Canvas, row, col int, segs []segment) {
	for _, s := range segs {
		c.WriteTextSkipSpaces(row, col, s.text, s.style)
		col += lipgloss.Width(s.text)
	}
}

func (m Model) blinkOn() bool {
	return (m.now.UnixMilli()/500)%2 == 0
}

// currentRunning/currentFlashing report the running/flashing state of
// whichever session m.mode is currently viewing — each of Pomodoro,
// Countdown, and Timer tracks these independently now (see model.go), since
// they're shared sessions that can run regardless of which screen any given
// window happens to be showing.
func (m Model) currentRunning() bool {
	switch m.mode {
	case modePomodoro:
		return m.pomoRunning
	case modeCountdown:
		return m.countdownRunning
	case modeTimer:
		return m.timerRunning
	}
	return false
}

func (m Model) currentFlashing() bool {
	switch m.mode {
	case modePomodoro:
		return m.pomoFlashing
	case modeCountdown:
		return m.countdownFlashing
	}
	return false
}

func (m Model) phaseLabel() string {
	switch m.mode {
	case modePomodoro:
		switch m.pomoPhase {
		case phaseWork:
			return "WORK"
		case phaseBreak:
			return "BREAK"
		default:
			return "LONG BREAK"
		}
	case modeCountdown:
		return "COUNTDOWN"
	case modeTimer:
		return "TIMER"
	default:
		return "CLOCK"
	}
}

func (m Model) phaseColor() lipgloss.Color {
	if m.mode == modePomodoro && m.pomoPhase != phaseWork {
		return m.theme.Accent
	}
	return m.theme.FG
}

func (m Model) bigTimeString() string {
	switch m.mode {
	case modePomodoro:
		return formatSec(liveRemaining(m.pomoRemaining, m.pomoRunning, m.pomoStartedAt, m.now))
	case modeCountdown:
		return formatSec(liveRemaining(m.countdownRemaining, m.countdownRunning, m.countdownStartedAt, m.now))
	case modeTimer:
		return formatSec(liveElapsed(m.timerElapsed, m.timerRunning, m.timerStartedAt, m.now))
	default:
		return m.now.Format("15:04:05")
	}
}

func (m Model) statusText() string {
	label := m.phaseLabel()
	if m.currentFlashing() {
		return label + " · DONE"
	}
	if m.mode == modeClock {
		return label
	}
	if m.currentRunning() {
		return label + " · RUNNING"
	}
	return label + " · PAUSED"
}

// pomoDotsSegments builds the row-of-roundsPerCycle progress dots (●/○)
// reflecting the Pomodoro session's own round count, independent of m.mode.
func (m Model) pomoDotsSegments() []segment {
	completed := (m.pomoRound - 1) % roundsPerCycle
	filled := cellStyle{fg: m.theme.FG}
	hollow := cellStyle{fg: m.theme.Dim}
	var segs []segment
	for i := 0; i < roundsPerCycle; i++ {
		if i > 0 {
			segs = append(segs, segment{text: "  "})
		}
		if i < completed {
			segs = append(segs, segment{text: "●", style: filled})
		} else {
			segs = append(segs, segment{text: "○", style: hollow})
		}
	}
	return segs
}

func (m Model) View() string {
	canvas := m.canvas
	canvas.Reset(m.width, m.height)
	renderWeather(canvas, m.weather, m.theme, m.now.Sub(m.startTime), m.weatherSeed, m.weatherCache)

	blinkHidden := m.currentFlashing() && !m.blinkOn()

	bigStyle := cellStyle{fg: m.theme.FG, bold: true}
	statusStyle := cellStyle{fg: m.phaseColor(), bold: true}
	if m.currentFlashing() {
		bigStyle = cellStyle{fg: m.theme.Danger, bold: true}
		statusStyle = cellStyle{fg: m.theme.Danger, bold: true}
	}

	var lines [][]segment

	for _, row := range BigTime(m.bigTimeString()) {
		text := row
		if blinkHidden {
			text = ""
		}
		lines = append(lines, []segment{{text: text, style: bigStyle}})
	}

	// Secondary/status/extra lines are always reserved, blank when not
	// applicable, so the header block is the same height regardless of mode
	// and doesn't shift the big time around. levelVeryCompact omits this
	// whole block and the footer, shrinking the total rendered height down
	// to just the digits.
	switch m.compactLevel {
	case levelFull:
		secondaryText := ""
		if m.mode == modeClock {
			secondaryText = strings.ToUpper(m.now.Format("Mon, Jan 2 2006"))
		}
		lines = append(lines, nil)
		lines = append(lines, []segment{{text: secondaryText, style: cellStyle{fg: m.theme.Dim}}})

		statusText := m.statusText()
		if blinkHidden {
			statusText = ""
		}
		lines = append(lines, []segment{{text: statusText, style: statusStyle}})

		var extraSegs []segment
		if m.mode == modePomodoro {
			extraSegs = m.pomoDotsSegments()
		} else if m.mode == modeCountdown && !m.countdownRunning {
			extraSegs = []segment{{text: "[ ↑ / ↓ ] adjust duration", style: cellStyle{fg: m.theme.Dim}}}
		}
		lines = append(lines, extraSegs)

	case levelCompact:
		// Exactly 2 lines below the digits, scoped to the current mode only
		// (unlike levelFull's secondary line, this isn't reserved-blank
		// across modes — each mode gets its own fixed pair of lines).
		lines = append(lines, nil)
		if m.mode == modeClock {
			dateText := strings.ToUpper(m.now.Format("Mon, Jan 2 2006"))
			clockText := m.now.Format("15:04:05")
			lines = append(lines, []segment{{text: dateText, style: cellStyle{fg: m.theme.Dim}}})
			lines = append(lines, []segment{{text: clockText, style: cellStyle{fg: m.theme.Dim2}}})
		} else {
			statusText := m.statusText()
			if blinkHidden {
				statusText = ""
			}
			lines = append(lines, []segment{{text: statusText, style: statusStyle}})

			var extraSegs []segment
			switch m.mode {
			case modePomodoro:
				extraSegs = m.pomoDotsSegments()
			case modeCountdown:
				extraSegs = []segment{{text: "[ ↑ / ↓ ] adjust duration", style: cellStyle{fg: m.theme.Dim}}}
			}
			lines = append(lines, extraSegs)
		}
	}

	footerSegs := m.footerSegments()
	totalHeight := len(lines)
	if m.showFooter && m.compactLevel == levelFull {
		totalHeight += 2 // blank gap + footer row
	}
	topOffset := (m.height - totalHeight) / 2
	if topOffset < 0 {
		topOffset = 0
	}

	for i, segs := range lines {
		row := topOffset + i
		writeSegments(canvas, row, centerCol(m.width, segWidth(segs)), segs)
	}

	if m.showFooter && m.compactLevel == levelFull {
		row := topOffset + len(lines) + 1
		writeSegments(canvas, row, centerCol(m.width, segWidth(footerSegs)), footerSegs)
	}

	return canvas.Render()
}

func (m Model) footerSegments() []segment {
	fg := cellStyle{fg: m.theme.FG}
	fgBold := cellStyle{fg: m.theme.FG, bold: true}
	dim := cellStyle{fg: m.theme.Dim}
	dim2 := cellStyle{fg: m.theme.Dim2}

	type btn struct {
		key   string
		label string
		mode  mode
	}
	btns := []btn{
		{"p", "pomodoro", modePomodoro},
		{"c", "countdown", modeCountdown},
		{"t", "timer", modeTimer},
		{"k", "clock", modeClock},
	}
	var left []segment
	for i, b := range btns {
		if i > 0 {
			left = append(left, segment{text: "  "})
		}
		text := "[" + b.key + "] " + b.label
		style := dim
		if b.mode == m.mode {
			style = fgBold
		}
		left = append(left, segment{text: text, style: style})
	}

	var right []segment
	if m.mode != modeClock {
		runLabel := "start"
		if m.currentRunning() {
			runLabel = "pause"
		}
		right = append(right, segment{text: "[space] " + runLabel, style: fg})
		right = append(right, segment{text: "  ", style: fg})
		right = append(right, segment{text: "[r] reset", style: fg})
		if m.mode == modePomodoro {
			right = append(right, segment{text: "  ", style: fg})
			right = append(right, segment{text: "[←/→] work/break", style: fg})
			right = append(right, segment{text: "  ", style: fg})
			right = append(right, segment{text: "[↑/↓] ±1 min", style: fg})
		}
		right = append(right, segment{text: "  ", style: fg})
		right = append(right, segment{text: "|", style: dim})
		right = append(right, segment{text: "  ", style: fg})
		right = append(right, segment{text: m.now.Format("15:04:05"), style: dim2})
	}

	contentWidth := footerContentWidth
	if m.width-4 > contentWidth {
		// keep bar at a readable fixed width, matching the prototype's
		// min(90vw, 640px) cap
	} else if m.width-4 > 20 {
		contentWidth = m.width - 4
	}

	leftW := segWidth(left)
	rightW := segWidth(right)
	pad := contentWidth - leftW - rightW
	if pad < 2 {
		pad = 2
	}

	segs := append([]segment{}, left...)
	segs = append(segs, segment{text: strings.Repeat(" ", pad)})
	segs = append(segs, right...)
	return segs
}
