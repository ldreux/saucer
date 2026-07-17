package app

import "os/exec"

// notifyPhaseComplete fires a best-effort macOS desktop notification when a
// Pomodoro phase finishes. It's intentionally fire-and-forget: exec's exit
// status/output is discarded so a slow or absent osascript binary (any
// non-macOS platform) can never stall or surface an error to the TUI.
func notifyPhaseComplete(finishedPhase phase) {
	title, body := notificationText(finishedPhase)
	script := `display notification "` + body + `" with title "` + title + `"`
	_ = exec.Command("osascript", "-e", script).Run()
}

func notificationText(finishedPhase phase) (title, body string) {
	switch finishedPhase {
	case phaseWork:
		return "Saucer", "Work session complete — time for a break."
	default: // phaseBreak, phaseLongBreak
		return "Saucer", "Break's over — back to work."
	}
}
