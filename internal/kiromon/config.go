package kiromon

import (
	"fmt"
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
	`!> `, // kiro-cli prompt contains "!> "
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
	Command       string `yaml:"command"`
	PromptPattern string `yaml:"prompt_pattern"`
	StartMsg      string `yaml:"start_msg"`
	EndMsg        string `yaml:"end_msg"`
}

// FileConfig represents the configuration file structure
type FileConfig struct {
	DefaultCommand string                  `yaml:"default_command"`
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


// defaultConfigContent is the default config file content
const defaultConfigContent = `# kiromon 設定ファイル
# 配置場所: ~/.config/kiromon/config.yaml

# デフォルトの通知コマンド
# default_command: notify-send

# ログファイルパス
# log_path: ~/kiromon.log

# コマンドごとのプリセット設定
# presets:
#   kiro-cli:
#     command: voicevox-speak-standalone
#     prompt_pattern: '!> '
#     start_msg: "{time}、タスクを開始したのだ"
#     end_msg: "{time}、タスクを終了したのだ。処理時間は、{duration}だったのだ。"
`

// initConfig creates the default config file
func initConfig() {
	configPath := getConfigPath()
	configDir := filepath.Dir(configPath)

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config file already exists: %s\n", configPath)
		return
	}

	// Create config directory
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating config directory: %v\n", err)
		os.Exit(1)
	}

	// Write default config
	if err := os.WriteFile(configPath, []byte(defaultConfigContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing config file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created config file: %s\n", configPath)
}
