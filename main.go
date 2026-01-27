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

const (
	StateRunning  = "running"
	StateWaiting  = "waiting"
	StateStopped  = "stopped"
)

// Prompt patterns for detecting input-waiting state
var defaultPromptPatterns = []string{
	`> ?$`,           // kiro-cli prompt ends with ">" or "> "
}

type Status struct {
	State         string    `json:"state"`
	Command       string    `json:"command"`
	PID           int       `json:"pid"`
	StartTime     time.Time `json:"start_time"`
	UpdatedAt     time.Time `json:"updated_at"`
	LastLines     []string  `json:"last_lines"`
	LastLine      string    `json:"last_line"`
	PromptMatched bool      `json:"prompt_matched"`
	IdleSeconds   float64   `json:"idle_seconds"`
}

var (
	statusFile     string
	screenBuffer   []string
	bufferMu       sync.RWMutex
	maxLines       = 100
	lastActivity   time.Time
	activityMu     sync.RWMutex
	currentLine    string
	currentLineMu  sync.RWMutex
	promptPatterns []*regexp.Regexp
)

func main() {
	// Cleanup stale files on startup
	cleanupStaleFiles()

	// Check mode: wrapper or status checker
	if len(os.Args) >= 2 && os.Args[1] == "-s" {
		showStatus()
		return
	}

	// Check for -p option (PID-only mode)
	if len(os.Args) >= 2 && os.Args[1] == "-p" {
		showStatusByPID()
		return
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  kiromon <command> [args...]       - Run command with monitoring")
		fmt.Fprintln(os.Stderr, "  kiromon -s <name>                 - Show status of all instances")
		fmt.Fprintln(os.Stderr, "  kiromon -s <name> -p <pid>        - Show status of specific PID")
		fmt.Fprintln(os.Stderr, "  kiromon -p <pid>                  - Show status by PID only")
		fmt.Fprintln(os.Stderr, "  kiromon -s <name> -d              - Daemon mode (monitor all instances)")
		fmt.Fprintln(os.Stderr, "  kiromon -p <pid> -d               - Daemon mode (monitor specific PID)")
		fmt.Fprintln(os.Stderr, "  kiromon -s <name> -d -i <sec>     - Set polling interval (default: 2s)")
		fmt.Fprintln(os.Stderr, "  kiromon -s <name> -d -c <cmd>     - Run command on state change")
		fmt.Fprintln(os.Stderr, "  kiromon -s <name> -d -c <cmd> -m <start> <end>  - Custom messages")
		fmt.Fprintln(os.Stderr, "  kiromon -s <name> -d -r <regex>   - Custom prompt pattern for waiting state")
		fmt.Fprintln(os.Stderr, "  kiromon -l                        - List all monitored processes")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  kiromon kiro-cli chat")
		fmt.Fprintln(os.Stderr, "  kiromon -s kiro-cli -d -c notify-send")
		fmt.Fprintln(os.Stderr, "  kiromon -p 12345 -d -c notify-send")
		fmt.Fprintln(os.Stderr, "  kiromon -s kiro-cli -d -c voicevox-speak -m \"開始\" \"完了\"")
		fmt.Fprintln(os.Stderr, "  kiromon -s kiro-cli -d -r '> ?$'  # Custom prompt pattern")
		os.Exit(1)
	}

	if os.Args[1] == "-l" {
		listProcesses()
		return
	}

	// Parse -p option is removed, just run wrapper
	runWrapper(os.Args[1:])
}

func getStatusDir() string {
	// 1. XDG_RUNTIME_DIR (Linux) - auto-cleaned on reboot
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		d := filepath.Join(dir, "kiromon")
		os.MkdirAll(d, 0700)
		return d
	}

	// 2. TMPDIR/kiromon-<uid> (Mac/Linux fallback)
	uid := os.Getuid()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("kiromon-%d", uid))
	os.MkdirAll(dir, 0700)
	return dir
}

// cleanupStaleFiles removes status files older than 24 hours or with dead processes
func cleanupStaleFiles() {
	dir := getStatusDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	now := time.Now()
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Remove files older than 24 hours
		if now.Sub(info.ModTime()) > 24*time.Hour {
			os.Remove(filePath)
			continue
		}

		// Remove files for dead processes
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var status Status
		if err := json.Unmarshal(data, &status); err != nil {
			os.Remove(filePath)
			continue
		}

		// Check if process is still running
		if err := syscall.Kill(status.PID, 0); err != nil {
			os.Remove(filePath)
		}
	}
}

func getStatusFile(name string) string {
	return filepath.Join(getStatusDir(), name+".json")
}

// getStatusFileWithPID returns status file path with PID for unique identification
func getStatusFileWithPID(name string, pid int) string {
	return filepath.Join(getStatusDir(), fmt.Sprintf("%s-%d.json", name, pid))
}

// findStatusFileByPID searches for a status file by PID
func findStatusFileByPID(pid int) (string, error) {
	dir := getStatusDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	
	suffix := fmt.Sprintf("-%d.json", pid)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), suffix) {
			return filepath.Join(dir, entry.Name()), nil
		}
	}
	
	return "", fmt.Errorf("no status file found for PID %d", pid)
}

// readStatusWithLock reads status file with shared lock
func readStatusWithLock(filename string) (*Status, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	
	// Acquire shared lock (allows multiple readers)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		return nil, err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	
	return &status, nil
}

func runWrapper(args []string) {
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
				ptmx.Write(buf[:n])
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

			os.Stdout.Write(buf[:n])

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
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			currentLineMu.RLock()
			line := currentLine
			currentLineMu.RUnlock()

			// Check if current line matches prompt pattern
			promptMatched := false
			for _, re := range promptPatterns {
				if re.MatchString(line) {
					promptMatched = true
					break
				}
			}
			// Also check if line ends with "> " or ">"
			if !promptMatched && (strings.HasSuffix(line, "> ") || strings.HasSuffix(line, ">")) {
				promptMatched = true
			}

			state := StateRunning
			if promptMatched {
				state = StateWaiting
			}
			updateStatus(state, strings.Join(args, " "), cmd.Process.Pid, line, promptMatched)
		}
	}()

	// Wait for command to finish
	err = cmd.Wait()
	updateStatus(StateStopped, strings.Join(args, " "), cmd.Process.Pid, "", false)

	// Cleanup: remove status file on exit
	os.Remove(statusFile)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func addLine(line string) {
	// Strip ANSI escape sequences
	line = stripAnsi(line)
	if line == "" {
		return
	}

	bufferMu.Lock()
	defer bufferMu.Unlock()

	screenBuffer = append(screenBuffer, line)
	if len(screenBuffer) > maxLines {
		screenBuffer = screenBuffer[len(screenBuffer)-maxLines:]
	}
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b\][^\x1b]*\x1b\\|\x1b[PX^_].*?(?:\x1b\\|\x07)|\x1b.|\?[0-9]+[hl]`)

func stripAnsi(s string) string {
	s = ansiRegex.ReplaceAllString(s, "")
	// Remove other control characters
	var result strings.Builder
	for _, r := range s {
		if r >= 32 && r < 127 {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

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
		StartTime:     time.Now(),
		UpdatedAt:     time.Now(),
		LastLines:     lines,
		LastLine:      lastLine,
		PromptMatched: promptMatched,
		IdleSeconds:   idle,
	}

	data, _ := json.MarshalIndent(status, "", "  ")
	atomicWriteFile(statusFile, data, 0600)
}

// atomicWriteFile writes data to a file atomically using temp file + rename
func atomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	
	// Create temp file in same directory (for same filesystem rename)
	tmp, err := os.CreateTemp(dir, ".kiromon-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	
	// Cleanup on failure
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()
	
	// Write data
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	
	// Sync to disk
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	
	if err := tmp.Close(); err != nil {
		return err
	}
	
	// Set permissions
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	
	// Atomic rename
	if err := os.Rename(tmpName, filename); err != nil {
		return err
	}
	
	tmpName = "" // Prevent cleanup since rename succeeded
	return nil
}

func showStatus() {
	var name string
	var pid int
	daemon := false
	interval := 2.0
	command := ""
	startMsg := "タスクを開始しました。"
	endMsg := "タスクを終了しました。"
	promptPattern := ""

	// Parse flags after -s
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-d":
			daemon = true
		case "-p":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				if _, err := fmt.Sscanf(args[i], "%d", &pid); err != nil {
					fmt.Fprintf(os.Stderr, "Invalid PID: %s\n", args[i])
					os.Exit(1)
				}
			} else {
				fmt.Fprintln(os.Stderr, "Error: -p requires a PID number")
				os.Exit(1)
			}
		case "-i":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				if _, err := fmt.Sscanf(args[i], "%f", &interval); err != nil {
					fmt.Fprintf(os.Stderr, "Invalid interval: %s\n", args[i])
					os.Exit(1)
				}
			} else {
				fmt.Fprintln(os.Stderr, "Error: -i requires a number (e.g., -i 2)")
				os.Exit(1)
			}
		case "-c":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				command = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "Error: -c requires a command")
				os.Exit(1)
			}
		case "-m":
			// Custom messages: -m "start message" "end message"
			if i+2 < len(args) && !strings.HasPrefix(args[i+1], "-") && !strings.HasPrefix(args[i+2], "-") {
				i++
				startMsg = args[i]
				i++
				endMsg = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "Error: -m requires two messages (e.g., -m \"Started\" \"Finished\")")
				os.Exit(1)
			}
		case "-r":
			// Custom prompt regex pattern
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				promptPattern = args[i]
				// Validate regex
				if _, err := regexp.Compile(promptPattern); err != nil {
					fmt.Fprintf(os.Stderr, "Invalid regex pattern: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Fprintln(os.Stderr, "Error: -r requires a regex pattern")
				os.Exit(1)
			}
		default:
			if !strings.HasPrefix(args[i], "-") && name == "" {
				name = args[i]
			}
		}
	}

	if name == "" {
		listProcesses()
		return
	}

	if daemon {
		runStatusDaemon(name, pid, interval, command, startMsg, endMsg, promptPattern)
	} else {
		showSingleStatus(name, pid)
	}
}

// showStatusByPID handles -p <pid> mode (without -s)
func showStatusByPID() {
	var pid int
	daemon := false
	interval := 2.0
	command := ""
	startMsg := "タスクを開始しました。"
	endMsg := "タスクを終了しました。"
	promptPattern := ""

	// Parse flags after -p
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-d":
			daemon = true
		case "-i":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				if _, err := fmt.Sscanf(args[i], "%f", &interval); err != nil {
					fmt.Fprintf(os.Stderr, "Invalid interval: %s\n", args[i])
					os.Exit(1)
				}
			}
		case "-c":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				command = args[i]
			}
		case "-m":
			if i+2 < len(args) && !strings.HasPrefix(args[i+1], "-") && !strings.HasPrefix(args[i+2], "-") {
				i++
				startMsg = args[i]
				i++
				endMsg = args[i]
			}
		case "-r":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				promptPattern = args[i]
			}
		default:
			if !strings.HasPrefix(args[i], "-") && pid == 0 {
				if _, err := fmt.Sscanf(args[i], "%d", &pid); err != nil {
					fmt.Fprintf(os.Stderr, "Invalid PID: %s\n", args[i])
					os.Exit(1)
				}
			}
		}
	}

	if pid == 0 {
		fmt.Fprintln(os.Stderr, "Error: -p requires a PID number")
		os.Exit(1)
	}

	// Find status file by PID
	filePath, err := findStatusFileByPID(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No status found for PID %d\n", pid)
		os.Exit(1)
	}

	// Extract name from filename (e.g., "kiro-cli-12345.json" -> "kiro-cli")
	baseName := filepath.Base(filePath)
	name := strings.TrimSuffix(baseName, fmt.Sprintf("-%d.json", pid))

	if daemon {
		runStatusDaemon(name, pid, interval, command, startMsg, endMsg, promptPattern)
	} else {
		status, err := readStatusWithLock(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading status: %v\n", err)
			os.Exit(1)
		}
		printStatus(name, status)
	}
}

func showSingleStatus(name string, pid int) {
	// If PID specified, look for exact file
	if pid > 0 {
		statusFile := getStatusFileWithPID(name, pid)
		status, err := readStatusWithLock(statusFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "No status found for '%s' with PID %d\n", name, pid)
			os.Exit(1)
		}
		printStatus(name, status)
		return
	}

	// Try exact match first (legacy format)
	statusFile := getStatusFile(name)
	status, err := readStatusWithLock(statusFile)
	if err == nil {
		printStatus(name, status)
		return
	}
	
	// Try to find files matching name-*.json pattern
	dir := getStatusDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No status found for '%s'\n", name)
		os.Exit(1)
	}
	
	prefix := name + "-"
	var found []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ".json") {
			found = append(found, filepath.Join(dir, entry.Name()))
		}
	}
	
	if len(found) == 0 {
		fmt.Fprintf(os.Stderr, "No status found for '%s'\n", name)
		os.Exit(1)
	}
	
	// Show all matching processes
	for _, f := range found {
		status, err := readStatusWithLock(f)
		if err != nil {
			continue
		}
		// Check if process is still alive
		if syscall.Kill(status.PID, 0) != nil {
			os.Remove(f)
			continue
		}
		printStatus(name, status)
		fmt.Println()
	}
}

func runStatusDaemon(name string, pid int, interval float64, command string, startMsg string, endMsg string, promptPattern string) {
	// Find all status files for this name
	dir := getStatusDir()
	
	// Compile custom prompt pattern if provided
	var customPromptRe *regexp.Regexp
	if promptPattern != "" {
		customPromptRe = regexp.MustCompile(promptPattern)
	}

	fmt.Printf("Monitoring %s", name)
	if pid > 0 {
		fmt.Printf(" (PID: %d)", pid)
	}
	fmt.Printf(" (interval: %.1fs)\n", interval)
	if promptPattern != "" {
		fmt.Printf("Prompt pattern: %s\n", promptPattern)
	}
	if command != "" {
		fmt.Printf("Command: %s\n", command)
		fmt.Printf("  Start: %q\n", startMsg)
		fmt.Printf("  End:   %q\n", endMsg)
	}
	fmt.Println(strings.Repeat("-", 50))

	ticker := time.NewTicker(time.Duration(interval * float64(time.Second)))
	defer ticker.Stop()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Track state per PID
	lastStates := make(map[int]string)

	checkStatus := func() {
		// If specific PID requested, only check that one
		if pid > 0 {
			filePath := getStatusFileWithPID(name, pid)
			status, err := readStatusWithLock(filePath)
			if err != nil {
				if lastStates[pid] != "not_found" {
					fmt.Printf("[%s] PID %d: not found\n", time.Now().Format("15:04:05"), pid)
					lastStates[pid] = "not_found"
				}
				return
			}
			
			// Check if process is still running
			if err := syscall.Kill(status.PID, 0); err != nil {
				if lastStates[pid] != "terminated" {
					fmt.Printf("[%s] PID %d terminated\n", time.Now().Format("15:04:05"), pid)
					lastStates[pid] = "terminated"
				}
				os.Remove(filePath)
				return
			}
			
			checkAndNotify(status, customPromptRe, lastStates, command, startMsg, endMsg)
			return
		}

		// Find all matching status files
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		
		prefix := name + "-"
		exactFile := name + ".json"
		foundAny := false
		
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			if entry.Name() != exactFile && !strings.HasPrefix(entry.Name(), prefix) {
				continue
			}
			
			filePath := filepath.Join(dir, entry.Name())
			status, err := readStatusWithLock(filePath)
			if err != nil {
				continue
			}
			
			// Check if process is still running
			if err := syscall.Kill(status.PID, 0); err != nil {
				if lastStates[status.PID] != "terminated" {
					fmt.Printf("[%s] PID %d terminated\n", time.Now().Format("15:04:05"), status.PID)
					lastStates[status.PID] = "terminated"
				}
				os.Remove(filePath)
				continue
			}
			
			foundAny = true
			checkAndNotify(status, customPromptRe, lastStates, command, startMsg, endMsg)
		}
		
		if !foundAny && len(lastStates) > 0 {
			// All processes gone
			for p, state := range lastStates {
				if state != "not_found" && state != "terminated" {
					fmt.Printf("[%s] PID %d: process not found\n", time.Now().Format("15:04:05"), p)
					lastStates[p] = "not_found"
				}
			}
		}
	}

	// Initial check
	checkStatus()

	for {
		select {
		case <-ticker.C:
			checkStatus()
		case <-sigCh:
			fmt.Println("\nStopped monitoring")
			return
		}
	}
}

func checkAndNotify(status *Status, customPromptRe *regexp.Regexp, lastStates map[int]string, command, startMsg, endMsg string) {
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
		if currentState == StateWaiting {
			message = endMsg
		} else if currentState == StateRunning && lastState == StateWaiting {
			message = startMsg
		}

		// Print state change
		stateIcon := "🔄"
		if currentState == StateWaiting {
			stateIcon = "⏳"
		}
		fmt.Printf("[%s] PID %d: %s %s\n", time.Now().Format("15:04:05"), status.PID, stateIcon, currentState)

		if message != "" && lastState != "" {
			fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), message)

			if command != "" {
				go func(msg string) {
					cmd := exec.Command(command, msg)
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					cmd.Run()
				}(message)
			}
		}

		lastStates[status.PID] = currentState
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func listProcesses() {
	dir := getStatusDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("No monitored processes found")
		return
	}

	// Group by command name
	type processInfo struct {
		status   *Status
		filePath string
	}
	groups := make(map[string][]processInfo)

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		status, err := readStatusWithLock(filePath)
		if err != nil {
			continue
		}

		// Check if process is still running
		if status.State != StateStopped {
			if err := syscall.Kill(status.PID, 0); err != nil {
				os.Remove(filePath)
				continue
			}
		}

		// Extract base name (remove -PID suffix if present)
		name := strings.TrimSuffix(entry.Name(), ".json")
		baseName := name
		if idx := strings.LastIndex(name, "-"); idx > 0 {
			// Check if suffix is a number (PID)
			if _, err := fmt.Sscanf(name[idx+1:], "%d", new(int)); err == nil {
				baseName = name[:idx]
			}
		}

		groups[baseName] = append(groups[baseName], processInfo{status: status, filePath: filePath})
	}

	if len(groups) == 0 {
		fmt.Println("No monitored processes found")
		return
	}

	fmt.Println("Monitored processes:")
	fmt.Println(strings.Repeat("-", 70))

	for name, processes := range groups {
		if len(processes) == 1 {
			p := processes[0]
			stateIcon := "🔄"
			if p.status.State == StateWaiting {
				stateIcon = "⏳"
			}
			fmt.Printf("%s %-20s PID:%-8d idle: %.1fs\n", stateIcon, name, p.status.PID, p.status.IdleSeconds)
		} else {
			// Multiple instances
			fmt.Printf("📦 %s (%d instances)\n", name, len(processes))
			for _, p := range processes {
				stateIcon := "🔄"
				if p.status.State == StateWaiting {
					stateIcon = "⏳"
				}
				fmt.Printf("   %s PID:%-8d idle: %.1fs\n", stateIcon, p.status.PID, p.status.IdleSeconds)
			}
		}
	}
}

func printStatus(name string, status *Status) {
	stateIcon := "⏹ STOPPED"
	switch status.State {
	case StateRunning:
		stateIcon = "🔄 RUNNING"
	case StateWaiting:
		stateIcon = "⏳ WAITING FOR INPUT"
	}

	fmt.Printf("=== %s: %s ===\n", name, stateIcon)
	fmt.Printf("Command: %s\n", status.Command)
	fmt.Printf("PID: %d\n", status.PID)
	fmt.Printf("Current line: %q\n", status.LastLine)
	fmt.Printf("Prompt matched: %v\n", status.PromptMatched)
	fmt.Printf("Idle: %.1f seconds\n", status.IdleSeconds)
	fmt.Printf("Updated: %s\n", status.UpdatedAt.Format("15:04:05"))
	fmt.Println()
	fmt.Println("--- Last Output ---")
	for _, line := range status.LastLines {
		fmt.Println(line)
	}
}
