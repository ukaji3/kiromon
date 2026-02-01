package kiromon

import (
	"regexp"
	"testing"
)

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain text", "hello world", "hello world"},
		{"with color", "\x1b[32mgreen\x1b[0m", "green"},
		{"with bold", "\x1b[1mbold\x1b[0m", "bold"},
		{"prompt with escape", "\x1b[?2004h> ", ">"},
		{"complex escapes", "\x1b[0;32m➜\x1b[0m \x1b[36mdir\x1b[0m", "dir"},
		{"empty string", "", ""},
		{"only escapes", "\x1b[0m\x1b[1m", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripAnsi(tt.input)
			if result != tt.expected {
				t.Errorf("stripAnsi(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMatchPrompt(t *testing.T) {
	defaultPatterns := compilePromptPatterns([]string{`> ?$`})

	tests := []struct {
		name           string
		line           string
		customPattern  *regexp.Regexp
		defaultPattern []*regexp.Regexp
		expected       bool
	}{
		{"ends with >", "prompt>", nil, defaultPatterns, true},
		{"ends with > space", "prompt> ", nil, defaultPatterns, true},
		{"matches default pattern", "test > ", nil, defaultPatterns, true},
		{"no match", "running...", nil, defaultPatterns, false},
		{"custom pattern match", "$ ", regexp.MustCompile(`\$ $`), nil, true},
		{"custom pattern no match", "> ", regexp.MustCompile(`\$ $`), nil, true}, // falls back to suffix check
		{"empty line", "", nil, defaultPatterns, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchPrompt(tt.line, tt.customPattern, tt.defaultPattern)
			if result != tt.expected {
				t.Errorf("matchPrompt(%q) = %v, want %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestCompilePromptPatterns(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		expected int
	}{
		{"single valid", []string{`> ?$`}, 1},
		{"multiple valid", []string{`> ?$`, `\$ $`}, 2},
		{"with invalid", []string{`> ?$`, `[invalid`}, 1}, // invalid pattern skipped
		{"empty", []string{}, 0},
		{"all invalid", []string{`[`, `(`}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compilePromptPatterns(tt.patterns)
			if len(result) != tt.expected {
				t.Errorf("compilePromptPatterns(%v) returned %d patterns, want %d", tt.patterns, len(result), tt.expected)
			}
		})
	}
}
