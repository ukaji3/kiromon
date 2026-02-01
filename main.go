package main

import "os"

func main() {
	// Cleanup stale files on startup
	cleanupStaleFiles()

	// Check for help options
	if len(os.Args) >= 2 && (os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "-help") {
		printUsage()
		return
	}

	// Check for -init option
	if len(os.Args) >= 2 && os.Args[1] == "-init" {
		initConfig()
		return
	}

	// Load config file and initialize prompt patterns
	config := loadConfig()
	if config != nil && len(config.PromptPatterns) > 0 {
		promptPatterns = compilePromptPatterns(config.PromptPatterns)
	} else {
		promptPatterns = compilePromptPatterns(DefaultPromptPatterns)
	}

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

	// Handle "--" as first argument
	if os.Args[1] == "--" {
		if len(os.Args) < 3 {
			printUsage()
			os.Exit(1)
		}
		runWrapper(os.Args[2:], nil)
		return
	}

	// Default: run wrapper mode
	runWrapper(os.Args[1:], nil)
}
