package main

import "os"

func main() {
	// Cleanup stale files on startup
	cleanupStaleFiles()

	// Initialize prompt patterns
	promptPatterns = compilePromptPatterns(DefaultPromptPatterns)

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
		printUsage()
		os.Exit(1)
	}

	if os.Args[1] == "-l" {
		listProcesses()
		return
	}

	// Check for standalone mode (-c option before command)
	if os.Args[1] == "-c" {
		runStandalone()
		return
	}

	// Default: run wrapper mode
	runWrapper(os.Args[1:], nil)
}
