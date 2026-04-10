package kiromon

import (
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

func TestIsRunningLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{"thinking", "Thinking...", true},
		{"running", "Running...", true},
		{"read", "Read", true},
		{"write", "Write", true},
		{"shell", "Shell", true},
		{"spinner thinking", "⠙ Thinking...", true},
		{"prompt", "16% !> What would you like to do next?", false},
		{"empty", "", false},
		{"normal output", "hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRunningLine(tt.line)
			if result != tt.expected {
				t.Errorf("isRunningLine(%q) = %v, want %v", tt.line, result, tt.expected)
			}
		})
	}
}
