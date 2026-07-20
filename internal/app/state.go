package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// PersistedState is the small slice of app state remembered across
// restarts: last-used mode, theme, background animation, and compact level.
// Nothing else (running timers) is persisted.
type PersistedState struct {
	Mode                string `json:"mode"`
	Theme               string `json:"theme"`
	BackgroundAnimation string `json:"backgroundAnimation"`
	CompactLevel        string `json:"compactLevel"`
}

func statePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "saucer", "state.json"), nil
}

// LoadState is best-effort: a missing file, unreadable file, or corrupt
// JSON, or an old file predating a field (which just zero-values that
// field, e.g. CompactLevel defaulting to "" and resolving to levelFull)
// all mean "no saved state for that part" — never a startup failure.
func LoadState() PersistedState {
	path, err := statePath()
	if err != nil {
		return PersistedState{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PersistedState{}
	}
	var s PersistedState
	if err := json.Unmarshal(data, &s); err != nil {
		return PersistedState{}
	}
	return s
}

// ApplyPersistedState applies a previously saved mode/background-animation/
// compact-level preference on top of a freshly constructed Model.
// Unrecognized or empty values are left at whatever New already set.
func (m *Model) ApplyPersistedState(s PersistedState) {
	if mo, ok := modeFromString(s.Mode); ok {
		m.mode = mo
	}
	if a, ok := bgAnimKindFromString(s.BackgroundAnimation); ok {
		m.bgAnim = a
	}
	if lvl, ok := compactLevelFromString(s.CompactLevel); ok {
		m.compactLevel = lvl
	}
}

// saveState is best-effort: any error (no config dir, permissions, disk
// full) is silently ignored — persistence is a nice-to-have and must never
// disrupt the running TUI.
func saveState(m Model) {
	path, err := statePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(PersistedState{
		Mode:                m.mode.String(),
		Theme:               m.theme.Name,
		BackgroundAnimation: m.bgAnim.String(),
		CompactLevel:        m.compactLevel.String(),
	})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

func modeFromString(s string) (mode, bool) {
	switch s {
	case "pomodoro":
		return modePomodoro, true
	case "countdown":
		return modeCountdown, true
	case "timer":
		return modeTimer, true
	case "clock":
		return modeClock, true
	}
	return modePomodoro, false
}

func bgAnimKindFromString(s string) (bgAnimKind, bool) {
	switch s {
	case "off":
		return bgAnimOff, true
	case "rain":
		return bgAnimRain, true
	case "snake":
		return bgAnimSnake, true
	case "tetris":
		return bgAnimTetris, true
	}
	return bgAnimOff, false
}

func compactLevelFromString(s string) (compactLevel, bool) {
	switch s {
	case "full":
		return levelFull, true
	case "compact":
		return levelCompact, true
	case "verycompact":
		return levelVeryCompact, true
	}
	return levelFull, false
}
