package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// exitCode stores the exit code to return after cleanup
var exitCode int

// Wrapper state variables
var (
	statusFile       string
	screenBuffer     []string
	bufferMu         sync.RWMutex
	lastActivity     time.Time
	activityMu       sync.RWMutex
	currentLine      string
	currentLineMu    sync.RWMutex
	promptPatterns   []*regexp.Regexp
	processStartTime time.Time
)

// runWrapper runs a command with PTY and monitors its state
func runWrapper(args []string, standalone *StandaloneConfig) {
	// Determine process name from command
	name := filepath.Base(args[0])

	// Create command
	cmd := exec.Command(args[0], args[1:]...)

	// Start with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting command: %v\n", err)
		os.Exit(1)
	}
	defer ptmx.Close()

	// Set status file with PID for unique identification
	statusFile = getStatusFileWithPID(name, cmd.Process.Pid)

	// Handle window size
	if term.IsTerminal(int(os.Stdin.Fd())) {
		// Set initial size
		if cols, rows, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
			pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
		}

		// Handle resize
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGWINCH)
		go func() {
			for range ch {
				if cols, rows, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
					pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
				}
			}
		}()
		ch <- syscall.SIGWINCH // Initial resize
		defer signal.Stop(ch)

		// Set raw mode
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			defer term.Restore(int(os.Stdin.Fd()), oldState)
		}
	}

	// Initialize status
	lastActivity = time.Now()
	processStartTime = time.Now()
	updateStatus(StateRunning, strings.Join(args, " "), cmd.Process.Pid, "", false)

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigCh {
			cmd.Process.Signal(sig)
		}
	}()

	// Copy stdin to pty (with activity tracking)
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				activityMu.Lock()
				lastActivity = time.Now()
				activityMu.Unlock()
				if _, err := ptmx.Write(buf[:n]); err != nil {
					return // PTY closed
				}
			}
		}
	}()

	// Copy pty to stdout (with buffering and activity tracking)
	go func() {
		buf := make([]byte, 4096)
		lineBuf := strings.Builder{}

		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				return
			}

			if _, err := os.Stdout.Write(buf[:n]); err != nil {
				return // stdout closed
			}

			activityMu.Lock()
			lastActivity = time.Now()
			activityMu.Unlock()

			// Process for line buffer (strip ANSI for storage)
			for i := 0; i < n; i++ {
				b := buf[i]
				if b == '\n' {
					line := lineBuf.String()
					if line != "" {
						addLine(line)
					}
					lineBuf.Reset()
				} else if b == '\r' {
					// Carriage return - might be clearing line or just CR
					// Don't reset, just update current line
					currentLineMu.Lock()
					currentLine = stripAnsi(lineBuf.String())
					currentLineMu.Unlock()
				} else {
					lineBuf.WriteByte(b)
				}
			}

			// Always update current line after processing buffer
			currentLineMu.Lock()
			currentLine = stripAnsi(lineBuf.String())
			currentLineMu.Unlock()
		}
	}()

	// Status updater
	var lastState string
	var lastNotifiedState string
	var stateChangeTime time.Time
	debounceDelay := time.Duration(DebounceDelay) * time.Second

	go func() {
		ticker := time.NewTicker(time.Duration(StatusInterval) * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			currentLineMu.RLock()
			line := currentLine
			currentLineMu.RUnlock()

			// Check if current line matches prompt pattern
			var customPattern *regexp.Regexp
			if standalone != nil {
				customPattern = standalone.PromptPattern
			}
			promptMatched := matchPrompt(line, customPattern, promptPatterns)

			state := StateRunning
			if promptMatched {
				state = StateWaiting
			}
			updateStatus(state, strings.Join(args, " "), cmd.Process.Pid, line, promptMatched)

			// Standalone mode: check for state changes and notify with debounce
			if standalone != nil {
				// Initialize lastState on first iteration
				if lastState == "" {
					lastState = state
					lastNotifiedState = state
					stateChangeTime = time.Now()
					// Initialize task start time
					standalone.TaskStartMu.Lock()
					standalone.TaskStartTime = time.Now()
					standalone.TaskStartMu.Unlock()
					stateIcon := "🔄"
					if state == StateWaiting {
						stateIcon = "⏳"
					}
					logToFile(standalone, "PID %d: %s %s (initial)", cmd.Process.Pid, stateIcon, state)
				} else if lastState != state {
					// State changed, reset debounce timer
					lastState = state
					stateChangeTime = time.Now()
				} else if lastState == state && lastNotifiedState != state {
					// State is stable, check if debounce delay has passed
					if time.Since(stateChangeTime) >= debounceDelay {
						var message string
						standalone.TaskStartMu.Lock()
						taskStart := standalone.TaskStartTime
						standalone.TaskStartMu.Unlock()

						if state == StateWaiting {
							// Check minimum duration before notifying
							taskDuration := time.Since(taskStart)
							if standalone.MinDuration > 0 && taskDuration < standalone.MinDuration {
								logToFile(standalone, "Skipping notification: duration %v < min %v", taskDuration, standalone.MinDuration)
								lastNotifiedState = state
								continue
							}
							message = replacePlaceholders(standalone.EndMsg, taskStart)
						} else if state == StateRunning {
							message = replacePlaceholders(standalone.StartMsg, taskStart)
							// Reset task start time for next cycle
							standalone.TaskStartMu.Lock()
							standalone.TaskStartTime = time.Now()
							standalone.TaskStartMu.Unlock()
						}

						stateIcon := "🔄"
						if state == StateWaiting {
							stateIcon = "⏳"
						}
						logToFile(standalone, "PID %d: %s %s", cmd.Process.Pid, stateIcon, state)

						if message != "" {
							logToFile(standalone, "%s", message)

							if standalone.Command != "" {
								go func(msg string, config *StandaloneConfig) {
									notifyCmd := exec.Command(config.Command, msg)
									output, err := notifyCmd.CombinedOutput()
									if err != nil {
										logToFile(config, "Command error: %v", err)
									}
									if len(output) > 0 {
										logToFile(config, "Command output: %s", strings.TrimSpace(string(output)))
									}
								}(message, standalone)
							}
						}

						lastNotifiedState = state
					}
				}
			}
		}
	}()

	// Wait for command to finish
	err = cmd.Wait()
	updateStatus(StateStopped, strings.Join(args, " "), cmd.Process.Pid, "", false)

	// Cleanup: remove status file on exit
	os.Remove(statusFile)

	// Close log file if standalone mode
	if standalone != nil && standalone.LogFile != nil {
		logToFile(standalone, "Process terminated")
		standalone.LogFile.Close()
	}

	// Store exit code but don't call os.Exit here (let defer run first)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
}

// addLine adds a line to the screen buffer
func addLine(line string) {
	// Strip ANSI escape sequences
	line = stripAnsi(line)
	if line == "" {
		return
	}

	bufferMu.Lock()
	defer bufferMu.Unlock()

	screenBuffer = append(screenBuffer, line)
	if len(screenBuffer) > MaxLines {
		screenBuffer = screenBuffer[len(screenBuffer)-MaxLines:]
	}
}

// updateStatus writes the current status to the status file
func updateStatus(state, command string, pid int, lastLine string, promptMatched bool) {
	bufferMu.RLock()
	lines := make([]string, len(screenBuffer))
	copy(lines, screenBuffer)
	bufferMu.RUnlock()

	activityMu.RLock()
	idle := time.Since(lastActivity).Seconds()
	activityMu.RUnlock()

	// Keep only last 20 lines for status
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}

	status := Status{
		State:         state,
		Command:       command,
		PID:           pid,
		StartTime:     processStartTime,
		UpdatedAt:     time.Now(),
		LastLines:     lines,
		LastLine:      lastLine,
		PromptMatched: promptMatched,
		IdleSeconds:   idle,
	}

	data, _ := json.MarshalIndent(status, "", "  ")
	atomicWriteFile(statusFile, data, 0600)
}
