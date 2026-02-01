package main

import (
	"testing"
)

func TestAddLine(t *testing.T) {
	// Reset buffer before test
	bufferMu.Lock()
	screenBuffer = nil
	bufferMu.Unlock()

	tests := []struct {
		name          string
		input         string
		expectedLen   int
		expectedLast  string
	}{
		{"plain text", "hello world", 1, "hello world"},
		{"with ANSI", "\x1b[32mgreen\x1b[0m", 2, "green"},
		{"empty after strip", "\x1b[0m", 2, "green"}, // no change, empty line ignored
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addLine(tt.input)

			bufferMu.RLock()
			defer bufferMu.RUnlock()

			if len(screenBuffer) != tt.expectedLen {
				t.Errorf("buffer length = %d, want %d", len(screenBuffer), tt.expectedLen)
			}
			if len(screenBuffer) > 0 && screenBuffer[len(screenBuffer)-1] != tt.expectedLast {
				t.Errorf("last line = %q, want %q", screenBuffer[len(screenBuffer)-1], tt.expectedLast)
			}
		})
	}
}

func TestAddLineMaxLines(t *testing.T) {
	// Reset buffer
	bufferMu.Lock()
	screenBuffer = nil
	bufferMu.Unlock()

	// Add more than MaxLines
	for i := 0; i < MaxLines+10; i++ {
		addLine("line")
	}

	bufferMu.RLock()
	defer bufferMu.RUnlock()

	if len(screenBuffer) != MaxLines {
		t.Errorf("buffer length = %d, want %d (MaxLines)", len(screenBuffer), MaxLines)
	}
}
