package kiromon

import (
	"regexp"
	"strings"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b\][^\x1b]*\x1b\\|\x1b[PX^_].*?(?:\x1b\\|\x07)|\x1b.|\?[0-9]+[hl]`)

// stripAnsi removes ANSI escape sequences and control characters from a string
func stripAnsi(s string) string {
	s = ansiRegex.ReplaceAllString(s, "")
	// Remove other control characters
	var result strings.Builder
	for _, r := range s {
		if r >= 32 && r < 127 {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

// runningKeywords are lines that indicate active processing
var runningKeywords = []string{"Thinking...", "Running...", "Read", "Write", "Shell", "Task"}

// isRunningLine returns true if the line contains a known active-processing keyword
func isRunningLine(line string) bool {
	for _, kw := range runningKeywords {
		if strings.Contains(line, kw) {
			return true
		}
	}
	return false
}

// promptMarker is the text shown in kiro-cli's input box when waiting for user input
const promptMarker = "ask a question or describe a task"

// workingMarker is the text shown in kiro-cli's TUI while actively processing
const workingMarker = "kiro is working"

// isPromptLine returns true if the line contains the input-waiting prompt marker
func isPromptLine(line string) bool {
	return strings.Contains(strings.ToLower(line), promptMarker)
}

// isWorkingLine returns true if the line contains the active-processing marker
func isWorkingLine(line string) bool {
	return strings.Contains(strings.ToLower(line), workingMarker)
}
