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

// weatherKind is a top-level ambient setting (like theme), cycled with 'w'.
type weatherKind int

const (
	weatherOff weatherKind = iota
	weatherRain
	weatherSnow
)

func nextWeather(w weatherKind) weatherKind {
	switch w {
	case weatherOff:
		return weatherRain
	case weatherRain:
		return weatherSnow
	default:
		return weatherOff
	}
}

func (w weatherKind) String() string {
	switch w {
	case weatherRain:
		return "rain"
	case weatherSnow:
		return "snow"
	default:
		return "off"
	}
}

var snowGlyphs = []rune{'*', '·', '.'}

// lighten blends a theme color toward white so the weather effect reads as
// a soft, pale background layer instead of competing with the foreground
// text (which uses the theme's colors at full strength).
func lighten(c lipgloss.Color, amount float64) lipgloss.Color {
	r, g, b := hexRGB(string(c))
	blend := func(v uint8) uint8 {
		return uint8(float64(v) + (255-float64(v))*amount)
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", blend(r), blend(g), blend(b)))
}

// darken blends a theme color toward black — used for the far end of the
// rain gradient so the fade has real visible contrast against the pale
// head, rather than staying within a narrow band of light tones.
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
// c2. Used to build a smooth multi-step gradient along the rain tail instead
// of a few hand-picked colors.
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

// weatherTuning holds the fixed per-kind constants, shared by weatherCache
// (which needs density/tailLen to size things correctly) and renderWeather
// (which needs all of it), so the two can't drift apart.
type weatherTuning struct {
	density    float64
	speed      float64 // rows per second
	tailLen    int
	headStyle  cellStyle
	tailStyles []cellStyle // fade steps after the head, index 0 = closest to head
}

// Rain's speed is tuned so a drop visibly steps to a new row often enough to
// read as smooth motion (roughly every 40-85ms — a text grid can't show
// sub-cell interpolation, so below that a "fall" reads as discrete jerky
// hops no matter how fast we redraw). Snow is deliberately very slow/gentle
// instead, with a fixed glyph per flake (no flicker/twinkle) — it's meant to
// read as calm drifting, not something drawing attention every frame.
func weatherTuningFor(kind weatherKind, theme Theme) weatherTuning {
	switch kind {
	case weatherRain:
		const rainTailLen = 6
		head := lighten(theme.FG, 0.55)
		// Faded end is pushed toward black (not just a less-lightened
		// light tone) so the gradient has real, easily visible contrast
		// from the pale head down to a genuinely dark tail tip.
		faded := darken(theme.Dim, 0.5)
		// gradient(...) includes the head color at index 0; the tail steps
		// (rendered after the head) are everything past that, so the whole
		// streak is one smooth fade from bright head to dim tail.
		steps := gradient(head, faded, rainTailLen)
		return weatherTuning{
			density:    1.0 / 6.0,
			speed:      10.0,
			tailLen:    rainTailLen,
			headStyle:  steps[0],
			tailStyles: steps[1:],
		}
	case weatherSnow:
		return weatherTuning{
			density:   1.0 / 10.0,
			speed:     2.0,
			tailLen:   1,
			headStyle: cellStyle{fg: lighten(theme.Dim2, 0.7)},
		}
	}
	return weatherTuning{}
}

// weatherColumnParams is the per-column identity derived from the PRNG: a
// pure function of (seed, column) that never changes frame to frame.
type weatherColumnParams struct {
	hasDrop       bool
	phaseOffset   float64
	speedVariance float64
	charVariant   int
}

// weatherCache memoizes weatherColumnParams so they're only (re)computed
// when the inputs that determine them actually change (weather kind, size,
// seed) rather than on every single render tick.
type weatherCache struct {
	kind          weatherKind
	width, height int
	seed          int64
	cols          []weatherColumnParams
}

func (wc *weatherCache) ensure(kind weatherKind, width, height int, seed int64, tuning weatherTuning) {
	if wc.kind == kind && wc.width == width && wc.height == height && wc.seed == seed {
		return
	}
	wc.kind, wc.width, wc.height, wc.seed = kind, width, height, seed

	loopLen := height + tuning.tailLen
	cols := make([]weatherColumnParams, width)
	for col := 0; col < width; col++ {
		rng := rand.New(rand.NewSource(seed ^ int64(col)*2654435761))
		cols[col] = weatherColumnParams{
			hasDrop:       rng.Float64() < tuning.density,
			phaseOffset:   rng.Float64() * float64(loopLen),
			speedVariance: 0.8 + rng.Float64()*0.4,
			charVariant:   rng.Intn(len(snowGlyphs)),
		}
	}
	wc.cols = cols
}

// renderWeather draws a deterministic, elapsed-time-driven falling-character
// effect into the background of the canvas, ported from the approach used by
// the tui-rain ratatui crate. Per-column identity (whether a column has a
// drop, its phase/speed variance) comes from cache — a pure function of
// (seed, column) — so only the elapsed-time-driven position math runs every
// frame.
func renderWeather(c *Canvas, kind weatherKind, theme Theme, elapsed time.Duration, seed int64, cache *weatherCache) {
	if kind == weatherOff || c.width == 0 || c.height == 0 {
		return
	}

	tuning := weatherTuningFor(kind, theme)
	cache.ensure(kind, c.width, c.height, seed, tuning)

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

		ch := rune('|')
		if kind == weatherSnow {
			// Fixed per-column glyph variant — no time-based cycling, so a
			// flake doesn't flicker between characters while it sits still.
			ch = snowGlyphs[p.charVariant]
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
			c.Set(row, col, ch, style)
		}
	}
}
