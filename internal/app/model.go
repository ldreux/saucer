package app

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	workDuration      = 25 * 60
	breakDuration     = 5 * 60
	longBreakDuration = 15 * 60
	roundsPerCycle    = 4

	countdownDefault = 10 * 60
	countdownStep    = 60
	countdownMin     = 60
	countdownMax     = 99 * 60

	flashDuration = 4 * time.Second
	tickInterval  = time.Second / 60 // ~60fps redraw, well within render headroom
)

type mode int

const (
	modePomodoro mode = iota
	modeCountdown
	modeTimer
	modeClock
)

func (m mode) String() string {
	switch m {
	case modePomodoro:
		return "pomodoro"
	case modeCountdown:
		return "countdown"
	case modeTimer:
		return "timer"
	case modeClock:
		return "clock"
	}
	return ""
}

// compactLevel controls how much of the display is shown, cycled with 'b'.
type compactLevel int

const (
	levelFull compactLevel = iota
	levelCompact
	levelVeryCompact
)

func nextCompactLevel(l compactLevel) compactLevel {
	switch l {
	case levelFull:
		return levelCompact
	case levelCompact:
		return levelVeryCompact
	default:
		return levelFull
	}
}

func (l compactLevel) String() string {
	switch l {
	case levelCompact:
		return "compact"
	case levelVeryCompact:
		return "verycompact"
	default:
		return "full"
	}
}

type phase int

const (
	phaseWork phase = iota
	phaseBreak
	phaseLongBreak
)

func (p phase) String() string {
	switch p {
	case phaseBreak:
		return "break"
	case phaseLongBreak:
		return "longbreak"
	default:
		return "work"
	}
}

func phaseFromString(s string) (phase, bool) {
	switch s {
	case "work":
		return phaseWork, true
	case "break":
		return phaseBreak, true
	case "longbreak":
		return phaseLongBreak, true
	}
	return phaseWork, false
}

// Model. mode is a purely local "which screen is this window showing"
// selector — it does NOT gate which session(s) are active. Pomodoro,
// Countdown, and Timer are three independent sessions, each shared live
// across every running instance via session.go (session.json): any window
// can start/pause/reset/adjust one regardless of which screen it currently
// displays, and any window viewing that mode shows the identical live
// value, derived fresh from a shared (snapshot, startedAt) pair rather than
// a locally-decremented counter — see liveRemaining/liveElapsed.
type Model struct {
	theme      Theme
	mode       mode
	showFooter   bool
	compactLevel compactLevel // full / compact / very compact — see compactLevel type
	weather      weatherKind

	now         time.Time
	startTime   time.Time
	weatherSeed int64

	// canvas and weatherCache are reused across frames (pointer fields, so
	// mutating through them persists across Model's value-receiver copies)
	// to avoid reallocating/recomputing on every render.
	canvas       *Canvas
	weatherCache *weatherCache

	pomoPhase     phase
	pomoRound     int
	pomoRemaining int // snapshot seconds, valid as of pomoStartedAt (or exact if !pomoRunning)
	pomoRunning   bool
	pomoStartedAt time.Time
	// pomoFlashing has no timeout: once a phase completes it blinks until the
	// user acknowledges it (space/reset/left/right), so a completion can
	// never be missed by looking away for a few seconds.
	pomoFlashing bool

	countdownRemaining  int
	countdownDuration   int
	countdownRunning    bool
	countdownStartedAt  time.Time
	countdownFlashing   bool
	countdownFlashUntil time.Time

	timerElapsed   int
	timerRunning   bool
	timerStartedAt time.Time

	// lastSessionMTime is the shared session file's last-observed modtime,
	// so the tick handler only pays to re-read+parse it when it actually
	// changed (a bare Stat every tick is cheap; a full read+adopt isn't
	// worth doing 60x/sec when nothing else wrote to it).
	lastSessionMTime time.Time

	width  int
	height int
}

type tickMsg time.Time

// New constructs a fresh Model for the given theme name (must be valid per
// IsValidTheme) and initial footer visibility.
func New(themeName string, showFooter bool) Model {
	now := time.Now()
	return Model{
		theme:              themeByName(themeName),
		mode:               modePomodoro,
		showFooter:         showFooter,
		now:                now,
		startTime:          now,
		weatherSeed:        now.UnixNano(),
		canvas:             NewCanvas(80, 24),
		weatherCache:       &weatherCache{},
		pomoPhase:          phaseWork,
		pomoRound:          1,
		pomoRemaining:      workDuration,
		countdownRemaining: countdownDefault,
		countdownDuration:  countdownDefault,
		timerElapsed:       0,
		width:              80,
		height:             24,
	}
}

// toSharedSession snapshots this model's three sessions for persisting to
// the shared session file.
func (m Model) toSharedSession() sharedSession {
	return sharedSession{
		Pomodoro: pomodoroSession{
			Phase:     m.pomoPhase.String(),
			Round:     m.pomoRound,
			Running:   m.pomoRunning,
			Remaining: m.pomoRemaining,
			StartedAt: m.pomoStartedAt,
			Flashing:  m.pomoFlashing,
		},
		Countdown: countdownSession{
			Remaining:  m.countdownRemaining,
			Duration:   m.countdownDuration,
			Running:    m.countdownRunning,
			StartedAt:  m.countdownStartedAt,
			Flashing:   m.countdownFlashing,
			FlashUntil: m.countdownFlashUntil,
		},
		Timer: timerSession{
			Elapsed:   m.timerElapsed,
			Running:   m.timerRunning,
			StartedAt: m.timerStartedAt,
		},
	}
}

// adoptSession merges a shared session snapshot into this model. Pomodoro
// and Countdown are only adopted when the snapshot shows evidence of real
// data (a valid phase string; a positive duration — countdownMin keeps a
// legitimate duration always > 0), so calling this against an empty/absent
// session file is a safe no-op rather than clobbering fresh defaults with
// zero values. Timer has no such "invalid" sentinel (zero elapsed, not
// running, is indistinguishable from "never started"), so it's always
// adopted — harmless, since that's a no-op against a fresh model anyway.
func (m *Model) adoptSession(s sharedSession) {
	if p, ok := phaseFromString(s.Pomodoro.Phase); ok {
		m.pomoPhase = p
		m.pomoRound = s.Pomodoro.Round
		m.pomoRemaining = s.Pomodoro.Remaining
		m.pomoRunning = s.Pomodoro.Running
		m.pomoStartedAt = s.Pomodoro.StartedAt
		m.pomoFlashing = s.Pomodoro.Flashing
	}
	if s.Countdown.Duration > 0 {
		m.countdownRemaining = s.Countdown.Remaining
		m.countdownDuration = s.Countdown.Duration
		m.countdownRunning = s.Countdown.Running
		m.countdownStartedAt = s.Countdown.StartedAt
		m.countdownFlashing = s.Countdown.Flashing
		m.countdownFlashUntil = s.Countdown.FlashUntil
	}
	m.timerElapsed = s.Timer.Elapsed
	m.timerRunning = s.Timer.Running
	m.timerStartedAt = s.Timer.StartedAt
}

// SyncSharedSession adopts whatever shared Pomodoro/Countdown/Timer session
// other running (or previously-run) instances left behind, so a fresh
// launch is consistent with what an already-running window would show,
// rather than only concurrent instances syncing while first-launch always
// resets to blank.
func (m *Model) SyncSharedSession() {
	m.adoptSession(loadSession())
	m.lastSessionMTime = sessionModTime()
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Init() tea.Cmd {
	return tickCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)

		if mt := sessionModTime(); mt.After(m.lastSessionMTime) {
			m.lastSessionMTime = mt
			m.adoptSession(loadSession())
		}

		changed, completedPhase, didComplete := m.checkCompletions()
		if changed {
			saveSession(m.toSharedSession())
		}
		if didComplete {
			return m, tea.Batch(tickCmd(), pomoCompleteCmd(completedPhase))
		}

		return m, tickCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// checkCompletions derives each running session's live value and reacts if
// it just crossed zero (Pomodoro phase transition; Countdown done), and
// clears Countdown's flash once its window has elapsed (Pomodoro's flash has
// no timeout — see pomoFlashing). Runs for all three sessions every tick
// regardless of which mode this window is displaying, since sessions run
// independently of the local view. Returns whether anything changed (so the
// caller knows whether to persist) and, if a Pomodoro phase completed on
// this tick in this window, that phase — so Update can fire the
// bell/notification exactly once, rather than on every tick a window merely
// observes pomoFlashing already set from another window's transition.
func (m *Model) checkCompletions() (changed bool, completedPhase phase, didComplete bool) {
	if m.pomoRunning && liveRemaining(m.pomoRemaining, true, m.pomoStartedAt, m.now) <= 0 {
		completedPhase = m.pomoPhase
		didComplete = true
		m.advancePomodoroPhase()
		changed = true
	}

	if m.countdownRunning && liveRemaining(m.countdownRemaining, true, m.countdownStartedAt, m.now) <= 0 {
		m.countdownRemaining = 0
		m.countdownRunning = false
		m.startCountdownFlash()
		changed = true
	}
	if m.countdownFlashing && !m.now.Before(m.countdownFlashUntil) {
		m.countdownFlashing = false
		changed = true
	}

	return changed, completedPhase, didComplete
}

func (m *Model) advancePomodoroPhase() {
	wasWork := m.pomoPhase == phaseWork
	if wasWork {
		isLong := m.pomoRound%roundsPerCycle == 0
		if isLong {
			m.pomoPhase = phaseLongBreak
			m.pomoRemaining = longBreakDuration
		} else {
			m.pomoPhase = phaseBreak
			m.pomoRemaining = breakDuration
		}
	} else {
		m.pomoPhase = phaseWork
		m.pomoRound++
		m.pomoRemaining = workDuration
	}
	m.pomoRunning = false
	m.startPomoFlash()
}

func (m *Model) startPomoFlash() {
	m.pomoFlashing = true
}

// pomoCompleteCmd fires the audible/visible "phase just completed" side
// effects — a terminal bell and a best-effort macOS desktop notification —
// exactly once, from the tea.Cmd Update() dispatches when checkCompletions
// detects a fresh Pomodoro phase transition. Bubble Tea always runs Cmds on
// their own goroutine, so neither of these can block the render/Update loop.
func pomoCompleteCmd(finishedPhase phase) tea.Cmd {
	return func() tea.Msg {
		fmt.Print("\a")
		notifyPhaseComplete(finishedPhase)
		return nil
	}
}

func (m *Model) startCountdownFlash() {
	m.countdownFlashing = true
	m.countdownFlashUntil = m.now.Add(flashDuration)
}

// toggleRun starts/pauses whichever session m.mode is currently viewing.
func (m *Model) toggleRun() {
	switch m.mode {
	case modePomodoro:
		if m.pomoRunning {
			m.pomoRemaining = liveRemaining(m.pomoRemaining, true, m.pomoStartedAt, m.now)
			m.pomoRunning = false
		} else {
			m.pomoStartedAt = m.now
			m.pomoRunning = true
		}
		m.pomoFlashing = false
	case modeCountdown:
		if m.countdownRunning {
			m.countdownRemaining = liveRemaining(m.countdownRemaining, true, m.countdownStartedAt, m.now)
			m.countdownRunning = false
		} else {
			m.countdownStartedAt = m.now
			m.countdownRunning = true
		}
		m.countdownFlashing = false
	case modeTimer:
		if m.timerRunning {
			m.timerElapsed = liveElapsed(m.timerElapsed, true, m.timerStartedAt, m.now)
			m.timerRunning = false
		} else {
			m.timerStartedAt = m.now
			m.timerRunning = true
		}
	}
}

// reset resets whichever session m.mode is currently viewing.
func (m *Model) reset() {
	switch m.mode {
	case modePomodoro:
		m.pomoPhase = phaseWork
		m.pomoRound = 1
		m.pomoRemaining = workDuration
		m.pomoRunning = false
		m.pomoFlashing = false
	case modeCountdown:
		m.countdownRemaining = m.countdownDuration
		m.countdownRunning = false
		m.countdownFlashing = false
	case modeTimer:
		m.timerElapsed = 0
		m.timerRunning = false
	}
}

func (m *Model) togglePhase() {
	if m.mode != modePomodoro {
		return
	}
	if m.pomoPhase == phaseWork {
		m.pomoPhase = phaseBreak
		m.pomoRemaining = breakDuration
	} else {
		m.pomoPhase = phaseWork
		m.pomoRemaining = workDuration
	}
	m.pomoRunning = false
	m.pomoFlashing = false
}

// setMode only changes which screen this window displays — it no longer
// touches any session's running/flash state, so switching your local view
// away from (say) a running Pomodoro doesn't pause it for anyone.
func (m *Model) setMode(mode mode) {
	m.mode = mode
}

func (m *Model) adjustCountdown(dir int) {
	if m.mode != modeCountdown || m.countdownRunning {
		return
	}
	d := m.countdownDuration + dir*countdownStep
	if d < countdownMin {
		d = countdownMin
	}
	if d > countdownMax {
		d = countdownMax
	}
	m.countdownDuration = d
	m.countdownRemaining = d
}

// adjustPomodoro nudges the current phase's remaining time by one minute in
// either direction, whether the timer is running or paused — unlike
// Countdown's duration (which only makes sense to edit while stopped), the
// Pomodoro counter is meant to be tweakable mid-session (e.g. "5 more
// minutes"). While running, resetting pomoStartedAt to now is required:
// pomoRemaining is a snapshot valid as of pomoStartedAt (see liveRemaining),
// so writing a new remaining value without also moving the snapshot instant
// would have the adjustment silently erased by already-elapsed time on the
// next tick.
func (m *Model) adjustPomodoro(dir int) {
	if m.mode != modePomodoro {
		return
	}
	r := liveRemaining(m.pomoRemaining, m.pomoRunning, m.pomoStartedAt, m.now) + dir*countdownStep
	if r < 0 {
		r = 0
	}
	if r > countdownMax {
		r = countdownMax
	}
	m.pomoRemaining = r
	if m.pomoRunning {
		m.pomoStartedAt = m.now
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	quit := false
	switch msg.String() {
	case "ctrl+c":
		quit = true
	case " ":
		m.toggleRun()
	case "r", "R":
		m.reset()
	case "p", "P":
		m.setMode(modePomodoro)
	case "c", "C":
		m.setMode(modeCountdown)
	case "t", "T":
		m.setMode(modeTimer)
	case "k", "K":
		m.setMode(modeClock)
	case "up", "+":
		m.adjustCountdown(1)
		m.adjustPomodoro(1)
	case "down", "-":
		m.adjustCountdown(-1)
		m.adjustPomodoro(-1)
	case "b", "B":
		m.compactLevel = nextCompactLevel(m.compactLevel)
	case "w", "W":
		m.weather = nextWeather(m.weather)
	case "tab":
		m.theme = nextTheme(m.theme.Name)
	case "shift+tab":
		m.theme = prevTheme(m.theme.Name)
	case "left", "right":
		m.togglePhase()
	case "q":
		quit = true
	}
	saveState(m)
	saveSession(m.toSharedSession())
	if quit {
		return m, tea.Quit
	}
	return m, nil
}

func formatSec(total int) string {
	if total < 0 {
		total = 0
	}
	h := total / 3600
	mm := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, mm, s)
	}
	return fmt.Sprintf("%02d:%02d", mm, s)
}
