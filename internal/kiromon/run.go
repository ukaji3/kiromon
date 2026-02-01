package kiromon

import "os"

// Run is the main entry point for kiromon
func Run() int {
	// Cleanup stale files on startup
	cleanupStaleFiles()

	// Check for help options
	if len(os.Args) >= 2 && (os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "-help") {
		printUsage()
		return 0
	}

	// Check for -init option
	if len(os.Args) >= 2 && os.Args[1] == "-init" {
		initConfig()
		return 0
	}

	// Initialize default prompt patterns
	promptPatterns = compilePromptPatterns(DefaultPromptPatterns)

	// Check mode: wrapper or status checker
	if len(os.Args) >= 2 && os.Args[1] == "-s" {
		showStatus()
		return 0
	}

	// Check for -p option (PID-only mode)
	if len(os.Args) >= 2 && os.Args[1] == "-p" {
		showStatusByPID()
		return 0
	}

	if len(os.Args) < 2 {
		printUsage()
		return 1
	}

	if os.Args[1] == "-l" {
		listProcesses()
		return 0
	}

	// Check for standalone mode (-c option before command)
	if os.Args[1] == "-c" {
		runStandalone()
		return exitCode
	}

	// Handle "--" as first argument
	if os.Args[1] == "--" {
		if len(os.Args) < 3 {
			printUsage()
			return 1
		}
		runWrapper(os.Args[2:], nil)
		return exitCode
	}

	// Default: run wrapper mode
	runWrapper(os.Args[1:], nil)
	return exitCode
}
