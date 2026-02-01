package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// replacePlaceholders replaces {time} and {duration} in message
func replacePlaceholders(msg string, taskStart time.Time) string {
	now := time.Now()

	// Replace {time} with current time in Japanese format
	msg = strings.ReplaceAll(msg, "{time}", formatTimeJapanese(now))

	// Replace {duration} with task duration
	if !taskStart.IsZero() {
		duration := now.Sub(taskStart)
		msg = strings.ReplaceAll(msg, "{duration}", formatDuration(duration))
	} else {
		msg = strings.ReplaceAll(msg, "{duration}", "0秒")
	}

	return msg
}

// formatTimeJapanese formats time in Japanese format (xx時xx分xx秒), omitting zero parts
func formatTimeJapanese(t time.Time) string {
	h, m, s := t.Hour(), t.Minute(), t.Second()

	var parts []string
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%d時", h))
	}
	if m > 0 || h > 0 {
		parts = append(parts, fmt.Sprintf("%d分", m))
	}
	parts = append(parts, fmt.Sprintf("%d秒", s))

	return strings.Join(parts, "")
}

// formatDuration formats duration in Japanese format (xx時間xx分xx秒), omitting zero parts
func formatDuration(d time.Duration) string {
	totalSeconds := int(d.Seconds())
	h := totalSeconds / 3600
	m := (totalSeconds % 3600) / 60
	s := totalSeconds % 60

	var parts []string
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%d時間", h))
	}
	if m > 0 || h > 0 {
		parts = append(parts, fmt.Sprintf("%d分", m))
	}
	parts = append(parts, fmt.Sprintf("%d秒", s))

	return strings.Join(parts, "")
}

// logToFile writes a log message to the config's log file
func logToFile(config *StandaloneConfig, format string, args ...interface{}) {
	if config == nil || config.LogFile == nil {
		return
	}
	config.LogMu.Lock()
	defer config.LogMu.Unlock()
	msg := fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
	config.LogFile.WriteString(msg)
}

// checkAndNotify checks for state changes and sends notifications
func checkAndNotify(status *Status, customPromptRe *regexp.Regexp, lastStates map[int]string, taskStartTimes map[int]time.Time, command, startMsg, endMsg string) {
	// Determine state using custom pattern if provided
	currentState := status.State
	if customPromptRe != nil {
		if customPromptRe.MatchString(status.LastLine) {
			currentState = StateWaiting
		} else {
			currentState = StateRunning
		}
	}

	lastState := lastStates[status.PID]

	// Detect state change
	if lastState != currentState {
		var message string
		taskStart := taskStartTimes[status.PID]

		if currentState == StateWaiting {
			message = replacePlaceholders(endMsg, taskStart)
		} else if currentState == StateRunning && lastState == StateWaiting {
			message = replacePlaceholders(startMsg, taskStart)
			// Reset task start time for next cycle
			taskStartTimes[status.PID] = time.Now()
		} else if currentState == StateRunning && lastState == "" {
			// First time seeing this PID in running state
			taskStartTimes[status.PID] = time.Now()
		}

		// Print state change
		stateIcon := "🔄"
		if currentState == StateWaiting {
			stateIcon = "⏳"
		}
		fmt.Printf("[%s] PID %d: %s %s\n", time.Now().Format("15:04:05"), status.PID, stateIcon, currentState)

		// Only execute command if message is not empty
		if message != "" && lastState != "" {
			fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), message)

			if command != "" {
				go func(msg string) {
					cmd := exec.Command(command, msg)
					cmd.Run()
				}(message)
			}
		}

		lastStates[status.PID] = currentState
	}
}
