package app

import (
	"math"
	"math/rand"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const (
	tetrisStepInterval = 500 * time.Millisecond
	// tetrisCols is fixed at the classic board width; cellW draws each board
	// cell as two terminal columns wide so blocks read as roughly square
	// instead of a tall, narrow sliver (a terminal cell is taller than it is
	// wide).
	tetrisCols  = 10
	tetrisCellW = 2
	// tetrisMaxCatchUp mirrors snakeMaxCatchUp — caps replay after a long
	// real-time gap so a resumed process doesn't stall redrawing.
	tetrisMaxCatchUp = 200
)

// tetrominoShape is one tetromino in one rotation: a fixed set of row,col
// offsets from its top-left bounding box.
type tetrominoShape struct {
	cells []point
	w, h  int
}

// tetrominoTypes holds every distinct rotation of each of the 7 tetrominoes
// (I, O, T, S, Z, J, L, in that order — index also selects the piece's
// color via tetrisPieceColor). S/Z only have 2 visually distinct rotations;
// the rest have 4 (O has 1). bestPlacement searches across whichever
// rotations a type has, so having fewer just narrows that type's options.
var tetrominoTypes = [][]tetrominoShape{
	{ // I
		{cells: []point{{0, 0}, {0, 1}, {0, 2}, {0, 3}}, w: 4, h: 1},
		{cells: []point{{0, 0}, {1, 0}, {2, 0}, {3, 0}}, w: 1, h: 4},
	},
	{ // O
		{cells: []point{{0, 0}, {0, 1}, {1, 0}, {1, 1}}, w: 2, h: 2},
	},
	{ // T
		{cells: []point{{0, 1}, {1, 0}, {1, 1}, {1, 2}}, w: 3, h: 2},
		{cells: []point{{0, 1}, {1, 1}, {1, 2}, {2, 1}}, w: 2, h: 3},
		{cells: []point{{0, 0}, {0, 1}, {0, 2}, {1, 1}}, w: 3, h: 2},
		{cells: []point{{0, 1}, {1, 0}, {1, 1}, {2, 1}}, w: 2, h: 3},
	},
	{ // S
		{cells: []point{{0, 1}, {0, 2}, {1, 0}, {1, 1}}, w: 3, h: 2},
		{cells: []point{{0, 0}, {1, 0}, {1, 1}, {2, 1}}, w: 2, h: 3},
	},
	{ // Z
		{cells: []point{{0, 0}, {0, 1}, {1, 1}, {1, 2}}, w: 3, h: 2},
		{cells: []point{{0, 1}, {1, 0}, {1, 1}, {2, 0}}, w: 2, h: 3},
	},
	{ // J
		{cells: []point{{0, 0}, {1, 0}, {1, 1}, {1, 2}}, w: 3, h: 2},
		{cells: []point{{0, 0}, {0, 1}, {1, 0}, {2, 0}}, w: 2, h: 3},
		{cells: []point{{0, 0}, {0, 1}, {0, 2}, {1, 2}}, w: 3, h: 2},
		{cells: []point{{0, 1}, {1, 1}, {2, 0}, {2, 1}}, w: 2, h: 3},
	},
	{ // L
		{cells: []point{{0, 2}, {1, 0}, {1, 1}, {1, 2}}, w: 3, h: 2},
		{cells: []point{{0, 0}, {1, 0}, {2, 0}, {2, 1}}, w: 2, h: 3},
		{cells: []point{{0, 0}, {0, 1}, {0, 2}, {1, 0}}, w: 3, h: 2},
		{cells: []point{{0, 0}, {0, 1}, {1, 1}, {2, 1}}, w: 2, h: 3},
	},
}

type tetrisBoardCell struct {
	filled bool
	color  lipgloss.Color
}

// tetrisState is a self-playing tetris simulation. Each spawn, a simple
// heuristic bot (see bestPlacement) picks the rotation and column that makes
// the resulting board lowest, flattest, and least holey — then the piece
// falls straight down into that spot with no further lateral movement.
// Lines clear when full; topping out clears the board and starts over,
// matching snake's reset-on-dead-end and rain's endless loop.
type tetrisState struct {
	width, height int
	seed          int64
	theme         Theme

	cols, rows int
	offsetCol  int
	board      [][]tetrisBoardCell

	rng                *rand.Rand
	pieceShape         tetrominoShape
	pieceRow, pieceCol int
	pieceColor         lipgloss.Color
	lastStep           time.Time
}

func (t *tetrisState) ensure(width, height int, seed int64, theme Theme) {
	t.theme = theme // cheap to keep current even when the size/seed guard below skips a full reset
	if t.width == width && t.height == height && t.seed == seed && t.board != nil {
		return
	}
	t.width, t.height, t.seed = width, height, seed
	t.cols = tetrisCols
	t.rows = height
	t.offsetCol = (width - t.cols*tetrisCellW) / 2
	t.rng = rand.New(rand.NewSource(seed))
	t.lastStep = time.Time{}

	if width < t.cols*tetrisCellW || height < 6 {
		t.board = nil
		return
	}
	t.board = newTetrisBoard(t.rows, t.cols)
	t.spawn()
}

func newTetrisBoard(rows, cols int) [][]tetrisBoardCell {
	board := make([][]tetrisBoardCell, rows)
	for r := range board {
		board[r] = make([]tetrisBoardCell, cols)
	}
	return board
}

func (t *tetrisState) advance(now time.Time) {
	if t.board == nil {
		return
	}
	if t.lastStep.IsZero() {
		t.lastStep = now
		return
	}
	steps := int(now.Sub(t.lastStep) / tetrisStepInterval)
	if steps <= 0 {
		return
	}
	if steps > tetrisMaxCatchUp {
		steps = tetrisMaxCatchUp
	}
	for i := 0; i < steps; i++ {
		t.step()
	}
	t.lastStep = t.lastStep.Add(time.Duration(steps) * tetrisStepInterval)
}

func (t *tetrisState) step() {
	if t.canMove(t.pieceRow+1, t.pieceCol, t.pieceShape) {
		t.pieceRow++
		return
	}
	lockOnBoard(t.board, t.rows, t.pieceShape, t.pieceRow, t.pieceCol, t.pieceColor)
	t.board, _ = clearFullLines(t.board, t.rows, t.cols)
	if t.toppedOut() {
		t.board = newTetrisBoard(t.rows, t.cols)
	}
	t.spawn()
}

func (t *tetrisState) canMove(row, col int, shape tetrominoShape) bool {
	return canMoveOnBoard(t.board, t.rows, t.cols, row, col, shape)
}

func (t *tetrisState) toppedOut() bool {
	for c := 0; c < t.cols; c++ {
		if t.board[0][c].filled {
			return true
		}
	}
	return false
}

func (t *tetrisState) spawn() {
	typeIdx := t.rng.Intn(len(tetrominoTypes))
	shape, col, ok := bestPlacement(t.board, t.rows, t.cols, tetrominoTypes[typeIdx])
	if !ok {
		// No legal placement at all (board has no room anywhere) — shouldn't
		// happen since toppedOut() clears the board before every spawn, but
		// fall back to a harmless default rather than an empty pieceShape.
		shape, col = tetrominoTypes[typeIdx][0], 0
	}
	t.pieceShape = shape
	t.pieceRow = -shape.h
	t.pieceCol = col
	t.pieceColor = tetrisPieceColor(t.theme, typeIdx)
}

// canMoveOnBoard reports whether shape can occupy (row, col) on board without
// going out of bounds or overlapping an already-filled cell. Cells above the
// board (r<0) never collide, so a piece can start fully off-screen and fall
// in. It's a free function (not a tetrisState method) so bestPlacement can
// run the same check against a scratch board copy during search.
func canMoveOnBoard(board [][]tetrisBoardCell, rows, cols, row, col int, shape tetrominoShape) bool {
	for _, off := range shape.cells {
		r, c := row+off.row, col+off.col
		if c < 0 || c >= cols || r >= rows {
			return false
		}
		if r < 0 {
			continue
		}
		if board[r][c].filled {
			return false
		}
	}
	return true
}

func lockOnBoard(board [][]tetrisBoardCell, rows int, shape tetrominoShape, row, col int, color lipgloss.Color) {
	for _, off := range shape.cells {
		r, c := row+off.row, col+off.col
		if r < 0 || r >= rows {
			continue
		}
		board[r][c] = tetrisBoardCell{filled: true, color: color}
	}
}

// clearFullLines removes every full row and shifts the rows above it down,
// returning a new board (bottom-up scratch slice, reversed at the end) plus
// how many rows were cleared.
func clearFullLines(board [][]tetrisBoardCell, rows, cols int) ([][]tetrisBoardCell, int) {
	kept := make([][]tetrisBoardCell, 0, rows)
	cleared := 0
	for r := rows - 1; r >= 0; r-- {
		full := true
		for c := 0; c < cols; c++ {
			if !board[r][c].filled {
				full = false
				break
			}
		}
		if full {
			cleared++
			continue
		}
		kept = append(kept, board[r])
	}
	for i := 0; i < cleared; i++ {
		kept = append(kept, make([]tetrisBoardCell, cols))
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return kept, cleared
}

func cloneBoard(board [][]tetrisBoardCell) [][]tetrisBoardCell {
	clone := make([][]tetrisBoardCell, len(board))
	for r, row := range board {
		clone[r] = append([]tetrisBoardCell(nil), row...)
	}
	return clone
}

// dropRow returns the row a piece dropped straight down at col would settle
// at (the same result the normal per-tick gravity in step() would arrive at,
// since nothing else changes the board between a spawn decision and the
// piece actually falling there) — or false if it doesn't fit at that column
// at all.
func dropRow(board [][]tetrisBoardCell, rows, cols int, shape tetrominoShape, col int) (int, bool) {
	if col < 0 || col+shape.w > cols {
		return 0, false
	}
	row := -shape.h
	for canMoveOnBoard(board, rows, cols, row+1, col, shape) {
		row++
	}
	return row, true
}

// Weights for a well-known simple 4-feature Tetris heuristic (aggregate
// height, complete lines, holes, bumpiness) that reliably favors flat,
// hole-free boards with frequent line clears over a long-running session —
// exactly what a background animation needs to still look good after
// playing for hours instead of degrading into a tall, holey mess.
const (
	tetrisWeightHeight    = -0.510066
	tetrisWeightLines     = 0.760666
	tetrisWeightHoles     = -0.35663
	tetrisWeightBumpiness = -0.184483
)

// bestPlacement searches every (rotation, column) combination for the given
// piece type and returns whichever placement scores highest once dropped and
// any resulting full lines are cleared.
func bestPlacement(board [][]tetrisBoardCell, rows, cols int, rotations []tetrominoShape) (tetrominoShape, int, bool) {
	bestScore := math.Inf(-1)
	var bestShape tetrominoShape
	bestCol := 0
	found := false

	for _, shape := range rotations {
		for col := 0; col <= cols-shape.w; col++ {
			row, ok := dropRow(board, rows, cols, shape, col)
			if !ok {
				continue
			}
			trial := cloneBoard(board)
			lockOnBoard(trial, rows, shape, row, col, "")
			afterClear, linesCleared := clearFullLines(trial, rows, cols)
			score := evaluateBoard(afterClear, rows, cols) + tetrisWeightLines*float64(linesCleared)
			if score > bestScore {
				bestScore, bestShape, bestCol, found = score, shape, col, true
			}
		}
	}
	return bestShape, bestCol, found
}

// evaluateBoard scores a board on aggregate column height, hole count, and
// bumpiness (adjacent-column height differences) — lower height/holes/
// bumpiness is better, so all three weights are negative.
func evaluateBoard(board [][]tetrisBoardCell, rows, cols int) float64 {
	heights := make([]int, cols)
	holes := 0
	for c := 0; c < cols; c++ {
		seenBlock := false
		for r := 0; r < rows; r++ {
			if board[r][c].filled {
				if !seenBlock {
					heights[c] = rows - r
					seenBlock = true
				}
			} else if seenBlock {
				holes++
			}
		}
	}

	aggHeight := 0
	for _, h := range heights {
		aggHeight += h
	}
	bumpiness := 0
	for c := 0; c < cols-1; c++ {
		d := heights[c] - heights[c+1]
		if d < 0 {
			d = -d
		}
		bumpiness += d
	}

	return tetrisWeightHeight*float64(aggHeight) + tetrisWeightHoles*float64(holes) + tetrisWeightBumpiness*float64(bumpiness)
}

// tetrisPieceColor spreads the 7 piece types across a gradient from a
// darkened FG to the theme's accent, so pieces are visually distinct without
// needing a hand-picked palette per theme.
func tetrisPieceColor(theme Theme, typeIdx int) lipgloss.Color {
	step := float64(typeIdx) / float64(len(tetrominoTypes)-1)
	return lerpColor(darken(theme.FG, 0.2), theme.Accent, step)
}

func renderTetris(c *Canvas, t *tetrisState) {
	if t.board == nil {
		return
	}
	for r := 0; r < t.rows; r++ {
		for col := 0; col < t.cols; col++ {
			cell := t.board[r][col]
			if !cell.filled {
				continue
			}
			drawTetrisBlock(c, r, t.offsetCol+col*tetrisCellW, cellStyle{fg: darken(cell.color, 0.15)})
		}
	}
	pieceStyle := cellStyle{fg: t.pieceColor, bold: true}
	for _, off := range t.pieceShape.cells {
		r, col := t.pieceRow+off.row, t.pieceCol+off.col
		if r < 0 || r >= t.rows {
			continue
		}
		drawTetrisBlock(c, r, t.offsetCol+col*tetrisCellW, pieceStyle)
	}
}

func drawTetrisBlock(c *Canvas, row, col int, style cellStyle) {
	c.Set(row, col, '█', style)
	c.Set(row, col+1, '█', style)
}
