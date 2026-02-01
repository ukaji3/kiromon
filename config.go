package main

import (
	"os"
	"regexp"
	"sync"
	"time"
)

// State constants
const (
	StateRunning = "running"
	StateWaiting = "waiting"
	StateStopped = "stopped"
)

// Default settings
const (
	MaxLines            = 100
	DebounceDelay       = 1   // seconds
	StatusInterval      = 500 // milliseconds
	DefaultPollInterval = 2.0 // seconds
)

// Default prompt patterns for detecting input-waiting state
var DefaultPromptPatterns = []string{
	`> ?$`, // kiro-cli prompt ends with ">" or "> "
}

// StandaloneConfig holds configuration for standalone mode
type StandaloneConfig struct {
	Command       string
	StartMsg      string
	EndMsg        string
	PromptPattern *regexp.Regexp
	LogFile       *os.File
	LogMu         sync.Mutex
	TaskStartTime time.Time
	TaskStartMu   sync.Mutex
}

