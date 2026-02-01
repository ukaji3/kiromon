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

// matchPrompt checks if the line matches any prompt pattern
func matchPrompt(line string, customPattern *regexp.Regexp, defaultPatterns []*regexp.Regexp) bool {
	// Check custom pattern first
	if customPattern != nil {
		if customPattern.MatchString(line) {
			return true
		}
	}

	// Check default patterns
	for _, re := range defaultPatterns {
		if re.MatchString(line) {
			return true
		}
	}

	// Also check if line ends with "> " or ">"
	if strings.HasSuffix(line, "> ") || strings.HasSuffix(line, ">") {
		return true
	}

	return false
}

// compilePromptPatterns compiles string patterns into regexp
func compilePromptPatterns(patterns []string) []*regexp.Regexp {
	var compiled []*regexp.Regexp
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			compiled = append(compiled, re)
		}
	}
	return compiled
}
