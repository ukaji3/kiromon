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
var runningKeywords = []string{"Thinking...", "Running...", "Read", "Write", "Shell"}

// isRunningLine returns true if the line contains a known active-processing keyword
func isRunningLine(line string) bool {
	for _, kw := range runningKeywords {
		if strings.Contains(line, kw) {
			return true
		}
	}
	return false
}
