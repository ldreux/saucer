package app

import (
	"math/rand"
	"time"
)

const (
	snakeStepInterval = 110 * time.Millisecond
	snakeInitialLen   = 4
	// snakeMaxLen caps how long the snake can grow. Past this it keeps
	// eating (food still relocates) but stops lengthening, so on a large
	// terminal it reads as a snake playing a bounded game rather than a
	// sprawling line that eventually fills the whole screen — and it keeps
	// chooseDirection's flood-fill cost (proportional to body length) from
	// growing without bound over a long-running session.
	snakeMaxLen = 32
	// snakeMaxCatchUp caps how many steps advance() replays in one call, so a
	// long real-time gap (e.g. the process was suspended) can't turn into a
	// multi-thousand-iteration stall on the next redraw — it just resumes at
	// a slightly stale point instead.
	snakeMaxCatchUp = 200
	// snakeCellW draws each game cell as two terminal columns wide, so a
	// segment reads as a square instead of a tall, narrow sliver (a
	// terminal cell is taller than it is wide) — same technique as
	// tetrisCellW in tetris.go.
	snakeCellW = 2
)

type point struct {
	row, col int
}

var snakeDirections = []point{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}

// snakeState is a self-playing snake simulation: a deterministic bot steers
// the head greedily toward the food, avoiding walls and its own body. It has
// no notion of score or player input — on any dead end it simply resets and
// starts over, forever, like rain looping.
type snakeState struct {
	width, height int // raw canvas size, used only by ensure()'s change-detection guard
	cols          int // game-grid column count (width / snakeCellW); rows == height, unscaled
	seed          int64

	body     []point        // body[0] is the head
	occupied map[point]bool // mirrors body as a set, so collides() is O(1) instead of an O(len(body)) scan — critical once the flood-fill safety check in chooseDirection calls it on every BFS neighbor, every step
	dir      point
	food     point
	rng      *rand.Rand
	lastStep time.Time

	// visitBuf/visitGen/bfsQueue are scratch state reused across every
	// floodFillFrom call (up to 3x per step) instead of allocating a fresh
	// map+slice each time. visitGen is a generation counter: bumping it and
	// comparing against visitBuf[idx] means "visited this call" without
	// having to clear the buffer first.
	visitBuf []int
	visitGen int
	bfsQueue []point
}

// ensure (re)initializes the simulation whenever the canvas size or seed
// changes (e.g. a terminal resize), mirroring rainCache's ensure pattern.
func (s *snakeState) ensure(width, height int, seed int64) {
	if s.width == width && s.height == height && s.seed == seed && s.body != nil {
		return
	}
	s.width, s.height, s.seed = width, height, seed
	s.cols = width / snakeCellW
	s.visitBuf = make([]int, s.cols*height)
	s.visitGen = 0
	s.reset()
}

func (s *snakeState) reset() {
	s.rng = rand.New(rand.NewSource(s.seed))
	s.lastStep = time.Time{}
	if s.cols < 6 || s.height < 6 {
		// Too small to place a snake — advance/render treat nil body as "skip".
		s.body = nil
		s.occupied = nil
		return
	}
	row, col := s.height/2, s.cols/2
	s.dir = point{0, 1}
	s.body = make([]point, 0, snakeInitialLen)
	s.occupied = make(map[point]bool, snakeInitialLen)
	for i := 0; i < snakeInitialLen; i++ {
		p := point{row, col - i}
		s.body = append(s.body, p)
		s.occupied[p] = true
	}
	s.placeFood()
}

func (s *snakeState) advance(now time.Time) {
	if s.body == nil {
		return
	}
	if s.lastStep.IsZero() {
		s.lastStep = now
		return
	}
	steps := int(now.Sub(s.lastStep) / snakeStepInterval)
	if steps <= 0 {
		return
	}
	if steps > snakeMaxCatchUp {
		steps = snakeMaxCatchUp
	}
	for i := 0; i < steps; i++ {
		s.step()
	}
	s.lastStep = s.lastStep.Add(time.Duration(steps) * snakeStepInterval)
}

func (s *snakeState) step() {
	s.chooseDirection()
	head := s.body[0]
	next := point{head.row + s.dir.row, head.col + s.dir.col}

	if s.collides(next) {
		// Ran itself into a corner — restart clean rather than getting stuck.
		s.reset()
		return
	}

	grew := next == s.food
	s.body = append([]point{next}, s.body...)
	s.occupied[next] = true
	if grew {
		s.placeFood()
	}
	if !grew || len(s.body) > snakeMaxLen {
		tail := s.body[len(s.body)-1]
		s.body = s.body[:len(s.body)-1]
		delete(s.occupied, tail)
	}
}

// chooseDirection steers toward the food but, unlike pure greedy distance
// minimization, first weighs how much open space each safe direction leads
// into (via floodFillFrom). A purely greedy bot routinely drives itself into
// pockets exactly its own size and dies — which looked, from the outside,
// like the snake randomly vanishing and resetting every second or two.
// Preferring directions with at least as much reachable space as the body is
// long avoids the vast majority of those self-inflicted dead ends, so resets
// become rare instead of near-constant.
func (s *snakeState) chooseDirection() {
	head := s.body[0]
	need := len(s.body)

	type candidate struct {
		dir       point
		safe      bool
		reachable int
		distance  int
	}
	var candidates []candidate
	for _, d := range snakeDirections {
		if d.row == -s.dir.row && d.col == -s.dir.col {
			continue // no immediate 180: that's always a self-collision anyway
		}
		next := point{head.row + d.row, head.col + d.col}
		if s.collides(next) {
			continue
		}
		reachable := s.floodFillFrom(next, need*2)
		candidates = append(candidates, candidate{
			dir:       d,
			safe:      reachable >= need,
			reachable: reachable,
			distance:  absInt(next.row-s.food.row) + absInt(next.col-s.food.col),
		})
	}
	if len(candidates) == 0 {
		return // truly no safe direction; step() will collide and reset
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		switch {
		case c.safe && !best.safe:
			best = c
		case c.safe == best.safe && c.distance < best.distance:
			best = c
		case c.safe == best.safe && c.distance == best.distance && c.reachable > best.reachable:
			best = c
		}
	}
	s.dir = best.dir
}

// floodFillFrom counts cells reachable from start by BFS through open space
// (treating the current body as a wall), stopping early once it reaches
// capN — chooseDirection only needs to know "enough room or not," not the
// exact size of the open area, so the cap keeps this cheap even on a large
// canvas. Uses the generation-counter visitBuf/bfsQueue scratch fields
// instead of allocating a fresh map+slice, since this runs up to 3x per
// step and a fresh map per call was the difference between this finishing
// instantly and stalling for minutes on a long-running snake.
func (s *snakeState) floodFillFrom(start point, capN int) int {
	if s.collides(start) {
		return 0
	}
	s.visitGen++
	gen := s.visitGen
	idx := start.row*s.cols + start.col
	s.visitBuf[idx] = gen
	s.bfsQueue = append(s.bfsQueue[:0], start)

	count := 0
	for i := 0; i < len(s.bfsQueue) && count < capN; i++ {
		p := s.bfsQueue[i]
		count++
		for _, d := range snakeDirections {
			next := point{p.row + d.row, p.col + d.col}
			if next.row < 0 || next.row >= s.height || next.col < 0 || next.col >= s.cols {
				continue
			}
			ni := next.row*s.cols + next.col
			if s.visitBuf[ni] == gen || s.occupied[next] {
				continue
			}
			s.visitBuf[ni] = gen
			s.bfsQueue = append(s.bfsQueue, next)
		}
	}
	return count
}

func (s *snakeState) collides(p point) bool {
	if p.row < 0 || p.row >= s.height || p.col < 0 || p.col >= s.cols {
		return true
	}
	return s.occupied[p]
}

func (s *snakeState) placeFood() {
	for attempt := 0; attempt < 1000; attempt++ {
		p := point{s.rng.Intn(s.height), s.rng.Intn(s.cols)}
		if !s.collides(p) {
			s.food = p
			return
		}
	}
	// Board's essentially full of snake — that's a win, not a bug. Restart.
	s.reset()
}

func renderSnake(c *Canvas, theme Theme, s *snakeState) {
	if s.body == nil {
		return
	}
	bodyStyle := cellStyle{fg: theme.Dim}
	headStyle := cellStyle{fg: theme.Dim2, bold: true}
	foodStyle := cellStyle{fg: theme.Accent}

	for i, p := range s.body {
		style := bodyStyle
		if i == 0 {
			style = headStyle
		}
		drawSnakeBlock(c, p.row, p.col, style)
	}
	c.Set(s.food.row, s.food.col*snakeCellW, '•', foodStyle)
}

func drawSnakeBlock(c *Canvas, row, col int, style cellStyle) {
	terminalCol := col * snakeCellW
	c.Set(row, terminalCol, '█', style)
	c.Set(row, terminalCol+1, '█', style)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
