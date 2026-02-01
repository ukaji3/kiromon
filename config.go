package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
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
	MinDuration   time.Duration
}

// PresetConfig holds preset configuration for a specific command
type PresetConfig struct {
	StartMsg string `yaml:"start_msg"`
	EndMsg   string `yaml:"end_msg"`
}

// FileConfig represents the configuration file structure
type FileConfig struct {
	DefaultCommand string                  `yaml:"default_command"`
	PromptPatterns []string                `yaml:"prompt_patterns"`
	LogPath        string                  `yaml:"log_path"`
	Presets        map[string]PresetConfig `yaml:"presets"`
}

// globalConfig holds the loaded configuration
var globalConfig *FileConfig

// getConfigPath returns the path to the config file
func getConfigPath() string {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "kiromon", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "kiromon", "config.yaml")
}

// loadConfig loads the configuration file
func loadConfig() *FileConfig {
	if globalConfig != nil {
		return globalConfig
	}

	configPath := getConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var config FileConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil
	}

	globalConfig = &config
	return globalConfig
}

// getPreset returns preset config for a command name
func getPreset(cmdName string) *PresetConfig {
	config := loadConfig()
	if config == nil || config.Presets == nil {
		return nil
	}
	if preset, ok := config.Presets[cmdName]; ok {
		return &preset
	}
	return nil
}

