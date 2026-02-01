package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// MonitorOptions holds common options for status monitoring
type MonitorOptions struct {
	Name          string
	PID           int
	Daemon        bool
	Interval      float64
	Command       string
	StartMsg      string
	EndMsg        string
	PromptPattern string
}

// parseMonitorOptions parses common monitor options from args
// Returns the options and the remaining unparsed arguments
func parseMonitorOptions(args []string) *MonitorOptions {
	opts := &MonitorOptions{
		Interval: DefaultPollInterval,
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-d":
			opts.Daemon = true
		case "-p":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				if _, err := fmt.Sscanf(args[i], "%d", &opts.PID); err != nil {
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
				if _, err := fmt.Sscanf(args[i], "%f", &opts.Interval); err != nil {
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
				opts.Command = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "Error: -c requires a command")
				os.Exit(1)
			}
		case "-ms":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				opts.StartMsg = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "Error: -ms requires a message")
				os.Exit(1)
			}
		case "-me":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				opts.EndMsg = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "Error: -me requires a message")
				os.Exit(1)
			}
		case "-r":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				opts.PromptPattern = args[i]
				// Validate regex
				if _, err := regexp.Compile(opts.PromptPattern); err != nil {
					fmt.Fprintf(os.Stderr, "Invalid regex pattern: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Fprintln(os.Stderr, "Error: -r requires a regex pattern")
				os.Exit(1)
			}
		default:
			// Non-option argument: treat as name or PID
			if !strings.HasPrefix(args[i], "-") && opts.Name == "" {
				opts.Name = args[i]
			}
		}
	}

	return opts
}

// printUsage prints the usage information
func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  kiromon <command> [args...]       - Run command with monitoring")
	fmt.Fprintln(os.Stderr, "  kiromon -s <name>                 - Show status of all instances")
	fmt.Fprintln(os.Stderr, "  kiromon -s <name> -p <pid>        - Show status of specific PID")
	fmt.Fprintln(os.Stderr, "  kiromon -p <pid>                  - Show status by PID only")
	fmt.Fprintln(os.Stderr, "  kiromon -s <name> -d              - Daemon mode (monitor all instances)")
	fmt.Fprintln(os.Stderr, "  kiromon -p <pid> -d               - Daemon mode (monitor specific PID)")
	fmt.Fprintln(os.Stderr, "  kiromon -s <name> -d -i <sec>     - Set polling interval (default: 2s)")
	fmt.Fprintln(os.Stderr, "  kiromon -s <name> -d -c <cmd>     - Run command on state change")
	fmt.Fprintln(os.Stderr, "  kiromon -s <name> -d -c <cmd> -ms <msg> -me <msg>  - Custom messages")
	fmt.Fprintln(os.Stderr, "  kiromon -s <name> -d -r <regex>   - Custom prompt pattern for waiting state")
	fmt.Fprintln(os.Stderr, "  kiromon -l                        - List all monitored processes")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Standalone mode (run + monitor in one process):")
	fmt.Fprintln(os.Stderr, "  kiromon -c <cmd> [-ms <msg>] [-me <msg>] [-r <regex>] [--] <command> [args...]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  -ms <msg>   Message for task start (running state)")
	fmt.Fprintln(os.Stderr, "  -me <msg>   Message for task end (waiting state)")
	fmt.Fprintln(os.Stderr, "              If omitted, no notification for that state")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Placeholders in messages:")
	fmt.Fprintln(os.Stderr, "  {time}      Current time (xx時xx分xx秒)")
	fmt.Fprintln(os.Stderr, "  {duration}  Task duration (xx時間xx分xx秒)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  kiromon kiro-cli chat")
	fmt.Fprintln(os.Stderr, "  kiromon -s kiro-cli -d -c notify-send")
	fmt.Fprintln(os.Stderr, "  kiromon -p 12345 -d -c notify-send")
	fmt.Fprintln(os.Stderr, "  kiromon -s kiro-cli -d -c voicevox-speak -ms \"開始\" -me \"完了\"")
	fmt.Fprintln(os.Stderr, "  kiromon -c notify-send -me \"完了\" kiro-cli chat  # End only")
	fmt.Fprintln(os.Stderr, "  kiromon -s kiro-cli -d -r '> ?$'  # Custom prompt pattern")
	fmt.Fprintln(os.Stderr, "  kiromon -c say -ms \"開始\" -me \"完了\" kiro-cli chat  # Standalone")
}

// runStandalone runs in standalone mode (wrapper + notification in one process)
func runStandalone() {
	command := ""
	startMsg := ""
	endMsg := ""
	promptPattern := ""
	var cmdArgs []string

	// First, get the command after -c (os.Args[1] is "-c")
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Error: -c requires a command")
		os.Exit(1)
	}
	command = os.Args[2]

	// Parse remaining options
	args := os.Args[3:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--":
			// Everything after -- is the command to run
			if i+1 < len(args) {
				cmdArgs = args[i+1:]
			}
			i = len(args) // break out of loop
		case "-ms":
			if i+1 < len(args) {
				i++
				startMsg = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "Error: -ms requires a message")
				os.Exit(1)
			}
		case "-me":
			if i+1 < len(args) {
				i++
				endMsg = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "Error: -me requires a message")
				os.Exit(1)
			}
		case "-r":
			if i+1 < len(args) {
				i++
				promptPattern = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "Error: -r requires a regex pattern")
				os.Exit(1)
			}
		default:
			// First non-option is the command to run
			if !strings.HasPrefix(args[i], "-") {
				cmdArgs = args[i:]
				i = len(args) // break out of loop
			}
		}
	}

	if command == "" {
		fmt.Fprintln(os.Stderr, "Error: -c <command> is required for standalone mode")
		os.Exit(1)
	}

	if len(cmdArgs) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no command specified to run")
		os.Exit(1)
	}

	// Open log file
	logFile, err := os.OpenFile("kiromon.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open kiromon.log: %v\n", err)
	}

	var promptRe *regexp.Regexp
	if promptPattern != "" {
		var err error
		promptRe, err = regexp.Compile(promptPattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid regex pattern: %v\n", err)
			os.Exit(1)
		}
	}

	config := &StandaloneConfig{
		Command:       command,
		StartMsg:      startMsg,
		EndMsg:        endMsg,
		PromptPattern: promptRe,
		LogFile:       logFile,
	}

	runWrapper(cmdArgs, config)
}

// showStatus handles -s mode (show status or daemon mode)
func showStatus() {
	opts := parseMonitorOptions(os.Args[2:])

	if opts.Name == "" {
		listProcesses()
		return
	}

	if opts.Daemon {
		runStatusDaemon(opts.Name, opts.PID, opts.Interval, opts.Command, opts.StartMsg, opts.EndMsg, opts.PromptPattern)
	} else {
		showSingleStatus(opts.Name, opts.PID)
	}
}

// showStatusByPID handles -p <pid> mode (without -s)
func showStatusByPID() {
	// Parse PID from first argument
	var pid int
	if len(os.Args) >= 3 && !strings.HasPrefix(os.Args[2], "-") {
		if _, err := fmt.Sscanf(os.Args[2], "%d", &pid); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid PID: %s\n", os.Args[2])
			os.Exit(1)
		}
	}

	// Parse remaining options
	opts := parseMonitorOptions(os.Args[2:])
	if pid != 0 {
		opts.PID = pid
	}

	if opts.PID == 0 {
		fmt.Fprintln(os.Stderr, "Error: -p requires a PID number")
		os.Exit(1)
	}

	// Find status file by PID
	filePath, err := findStatusFileByPID(opts.PID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No status found for PID %d\n", opts.PID)
		os.Exit(1)
	}

	// Extract name from filename (e.g., "kiro-cli-12345.json" -> "kiro-cli")
	baseName := filepath.Base(filePath)
	name := strings.TrimSuffix(baseName, fmt.Sprintf("-%d.json", opts.PID))

	if opts.Daemon {
		runStatusDaemon(name, opts.PID, opts.Interval, opts.Command, opts.StartMsg, opts.EndMsg, opts.PromptPattern)
	} else {
		status, err := readStatusWithLock(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading status: %v\n", err)
			os.Exit(1)
		}
		printStatus(name, status)
	}
}

// showSingleStatus shows status for a single process or all matching processes
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

// runStatusDaemon runs in daemon mode, monitoring status files
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
	taskStartTimes := make(map[int]time.Time)

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

			checkAndNotify(status, customPromptRe, lastStates, taskStartTimes, command, startMsg, endMsg)
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
			checkAndNotify(status, customPromptRe, lastStates, taskStartTimes, command, startMsg, endMsg)
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

// listProcesses lists all monitored processes
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

// printStatus prints the status of a process
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
