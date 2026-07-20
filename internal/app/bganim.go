package app

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// bgAnimKind is a top-level ambient setting (like theme), cycled with 'w'.
type bgAnimKind int

const (
	bgAnimOff bgAnimKind = iota
	bgAnimRain
	bgAnimSnake
	bgAnimTetris
)

func nextBgAnim(a bgAnimKind) bgAnimKind {
	switch a {
	case bgAnimOff:
		return bgAnimRain
	case bgAnimRain:
		return bgAnimSnake
	case bgAnimSnake:
		return bgAnimTetris
	default:
		return bgAnimOff
	}
}

func (a bgAnimKind) String() string {
	switch a {
	case bgAnimRain:
		return "rain"
	case bgAnimSnake:
		return "snake"
	case bgAnimTetris:
		return "tetris"
	default:
		return "off"
	}
}

// bgAnimState bundles the mutable, cross-frame state for every background
// animation. It's held behind a pointer field on Model (like canvas) so
// mutations persist across Model's value-receiver copies. Rain only ever
// touches rainCache; snake/tetris are actual stepped simulations and only
// touch their own field — whichever kind isn't selected just sits idle.
type bgAnimState struct {
	rainCache *rainCache
	snake     *snakeState
	tetris    *tetrisState
}

func newBgAnimState() *bgAnimState {
	return &bgAnimState{
		rainCache: &rainCache{},
		snake:     &snakeState{},
		tetris:    &tetrisState{},
	}
}

// renderBgAnim draws the currently selected ambient background effect into
// the canvas. Rain is purely a function of elapsed time (no persistent
// simulation state beyond per-column identity); snake and tetris are actual
// stepped simulations, so they're first advanced to "now" and then rendered
// from whatever state that leaves them in.
func renderBgAnim(c *Canvas, kind bgAnimKind, theme Theme, now time.Time, elapsed time.Duration, seed int64, state *bgAnimState) {
	if kind == bgAnimOff || c.width == 0 || c.height == 0 {
		return
	}
	switch kind {
	case bgAnimRain:
		renderRain(c, theme, elapsed, seed, state.rainCache)
	case bgAnimSnake:
		state.snake.ensure(c.width, c.height, seed)
		state.snake.advance(now)
		renderSnake(c, theme, state.snake)
	case bgAnimTetris:
		state.tetris.ensure(c.width, c.height, seed, theme)
		state.tetris.advance(now)
		renderTetris(c, state.tetris)
	}
}

// lighten blends a theme color toward white so an ambient effect reads as a
// soft, pale background layer instead of competing with the foreground text
// (which uses the theme's colors at full strength).
func lighten(c lipgloss.Color, amount float64) lipgloss.Color {
	r, g, b := hexRGB(string(c))
	blend := func(v uint8) uint8 {
		return uint8(float64(v) + (255-float64(v))*amount)
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", blend(r), blend(g), blend(b)))
}

// darken blends a theme color toward black — used for the far end of the
// rain gradient (and other ambient effects) so a fade has real visible
// contrast against a pale head, rather than staying within a narrow band of
// light tones.
func darken(c lipgloss.Color, amount float64) lipgloss.Color {
	r, g, b := hexRGB(string(c))
	blend := func(v uint8) uint8 {
		return uint8(float64(v) * (1 - amount))
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", blend(r), blend(g), blend(b)))
}

func hexRGB(hex string) (r, g, b uint8) {
	hex = strings.TrimPrefix(hex, "#")
	v, err := strconv.ParseUint(hex, 16, 32)
	if len(hex) != 6 || err != nil {
		return 255, 255, 255
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v)
}

// lerpColor linearly interpolates between two hex colors: t=0 is c1, t=1 is
// c2. Used to build smooth multi-step gradients (rain's tail fade, tetris
// piece colors) instead of a few hand-picked colors.
func lerpColor(c1, c2 lipgloss.Color, t float64) lipgloss.Color {
	r1, g1, b1 := hexRGB(string(c1))
	r2, g2, b2 := hexRGB(string(c2))
	lerp := func(a, b uint8) uint8 {
		return uint8(float64(a) + (float64(b)-float64(a))*t)
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", lerp(r1, r2), lerp(g1, g2), lerp(b1, b2)))
}

// gradient returns n evenly-spaced colors from c1 (index 0) to c2 (index
// n-1) inclusive.
func gradient(c1, c2 lipgloss.Color, n int) []cellStyle {
	steps := make([]cellStyle, n)
	for i := 0; i < n; i++ {
		t := 0.0
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		steps[i] = cellStyle{fg: lerpColor(c1, c2, t)}
	}
	return steps
}

// rainTuning holds rain's fixed tuning constants, shared by rainCache (which
// needs density/tailLen to size things correctly) and renderRain (which
// needs all of it), so the two can't drift apart.
type rainTuning struct {
	density    float64
	speed      float64 // rows per second
	tailLen    int
	headStyle  cellStyle
	tailStyles []cellStyle // fade steps after the head, index 0 = closest to head
}

// Rain's speed is tuned so a drop visibly steps to a new row often enough to
// read as smooth motion (roughly every 40-85ms — a text grid can't show
// sub-cell interpolation, so below that a "fall" reads as discrete jerky
// hops no matter how fast we redraw).
func rainTuningFor(theme Theme) rainTuning {
	const rainTailLen = 6
	head := lighten(theme.FG, 0.55)
	// Faded end is pushed toward black (not just a less-lightened light
	// tone) so the gradient has real, easily visible contrast from the pale
	// head down to a genuinely dark tail tip.
	faded := darken(theme.Dim, 0.5)
	// gradient(...) includes the head color at index 0; the tail steps
	// (rendered after the head) are everything past that, so the whole
	// streak is one smooth fade from bright head to dim tail.
	steps := gradient(head, faded, rainTailLen)
	return rainTuning{
		density:    1.0 / 6.0,
		speed:      10.0,
		tailLen:    rainTailLen,
		headStyle:  steps[0],
		tailStyles: steps[1:],
	}
}

// rainColumnParams is the per-column identity derived from the PRNG: a pure
// function of (seed, column) that never changes frame to frame.
type rainColumnParams struct {
	hasDrop       bool
	phaseOffset   float64
	speedVariance float64
}

// rainCache memoizes rainColumnParams so they're only (re)computed when the
// inputs that determine them actually change (size, seed) rather than on
// every single render tick.
type rainCache struct {
	width, height int
	seed          int64
	cols          []rainColumnParams
}

func (rc *rainCache) ensure(width, height int, seed int64, tuning rainTuning) {
	if rc.width == width && rc.height == height && rc.seed == seed {
		return
	}
	rc.width, rc.height, rc.seed = width, height, seed

	loopLen := height + tuning.tailLen
	cols := make([]rainColumnParams, width)
	for col := 0; col < width; col++ {
		rng := rand.New(rand.NewSource(seed ^ int64(col)*2654435761))
		cols[col] = rainColumnParams{
			hasDrop:       rng.Float64() < tuning.density,
			phaseOffset:   rng.Float64() * float64(loopLen),
			speedVariance: 0.8 + rng.Float64()*0.4,
		}
	}
	rc.cols = cols
}

// renderRain draws a deterministic, elapsed-time-driven falling-character
// effect into the background of the canvas, ported from the approach used by
// the tui-rain ratatui crate. Per-column identity (whether a column has a
// drop, its phase/speed variance) comes from cache — a pure function of
// (seed, column) — so only the elapsed-time-driven position math runs every
// frame.
func renderRain(c *Canvas, theme Theme, elapsed time.Duration, seed int64, cache *rainCache) {
	tuning := rainTuningFor(theme)
	cache.ensure(c.width, c.height, seed, tuning)

	elapsedSec := elapsed.Seconds()
	loopLen := c.height + tuning.tailLen

	for col, p := range cache.cols {
		if !p.hasDrop {
			continue
		}

		headRowF := tuning.speed*p.speedVariance*elapsedSec + p.phaseOffset
		headRow := int(math.Floor(headRowF)) % loopLen
		if headRow < 0 {
			headRow += loopLen
		}

		for i := 0; i < tuning.tailLen; i++ {
			row := headRow - i
			if row < 0 || row >= c.height {
				continue
			}
			style := tuning.headStyle
			if i > 0 && i-1 < len(tuning.tailStyles) {
				style = tuning.tailStyles[i-1]
			}
			c.Set(row, col, '|', style)
		}
	}
}
