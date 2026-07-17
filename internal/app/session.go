package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// pomodoroSession, countdownSession, and timerSession are stored as a
// snapshot value plus the wall-clock instant it's valid from, rather than a
// live-decrementing counter. Any process reading the same
// (Remaining/Elapsed, Running, StartedAt) triple derives the identical live
// value from its own clock — nothing is copied or interpolated between
// processes, so sync is exact by construction, not approximated.
type pomodoroSession struct {
	Phase     string    `json:"phase"`
	Round     int       `json:"round"`
	Running   bool      `json:"running"`
	Remaining int       `json:"remaining"`
	StartedAt time.Time `json:"started_at"`
	Flashing  bool      `json:"flashing"`
}

type countdownSession struct {
	Remaining  int       `json:"remaining"`
	Duration   int       `json:"duration"`
	Running    bool      `json:"running"`
	StartedAt  time.Time `json:"started_at"`
	Flashing   bool      `json:"flashing"`
	FlashUntil time.Time `json:"flash_until"`
}

// timerSession has no completion/flash concept — a stopwatch never finishes.
type timerSession struct {
	Elapsed   int       `json:"elapsed"`
	Running   bool      `json:"running"`
	StartedAt time.Time `json:"started_at"`
}

// sharedSession is the live, shared state across every currently-running
// instance. It's intentionally separate from persistedState/state.json:
// that file holds next-launch preferences (read once at startup, written on
// keypress); this one is polled continuously while the app runs.
type sharedSession struct {
	Pomodoro  pomodoroSession  `json:"pomodoro"`
	Countdown countdownSession `json:"countdown"`
	Timer     timerSession     `json:"timer"`
}

func sessionPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "saucer", "session.json"), nil
}

// loadSession is best-effort: a missing file, unreadable file, or corrupt
// JSON all just mean "no shared session yet" — never a failure.
func loadSession() sharedSession {
	path, err := sessionPath()
	if err != nil {
		return sharedSession{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return sharedSession{}
	}
	var s sharedSession
	if err := json.Unmarshal(data, &s); err != nil {
		return sharedSession{}
	}
	return s
}

// sessionModTime lets callers cheaply poll for external changes (a Stat)
// without paying to read+parse the file on every tick — only when the mtime
// actually moves does the caller bother calling loadSession.
func sessionModTime() time.Time {
	path, err := sessionPath()
	if err != nil {
		return time.Time{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// saveSession writes atomically (temp file + rename) rather than a direct
// os.WriteFile: this file is written far more often than state.json (every
// completion/transition, not just keypresses) and may be read concurrently
// by other running instances, so a reader must never observe a torn write.
// Rename is atomic on POSIX filesystems.
func saveSession(s sharedSession) {
	path, err := sessionPath()
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, "session-*.json.tmp")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return
	}
	_ = os.Rename(tmpPath, path)
}

// liveRemaining derives the current remaining seconds for a countdown-style
// session (Pomodoro phase or Countdown), clamped to zero.
func liveRemaining(remaining int, running bool, startedAt, now time.Time) int {
	if !running {
		return remaining
	}
	r := remaining - int(now.Sub(startedAt).Seconds())
	if r < 0 {
		return 0
	}
	return r
}

// liveElapsed is the count-up equivalent, used by the Timer/stopwatch.
func liveElapsed(elapsed int, running bool, startedAt, now time.Time) int {
	if !running {
		return elapsed
	}
	return elapsed + int(now.Sub(startedAt).Seconds())
}
