package kiromon

import (
	"strings"
	"testing"
	"time"
)

func TestFormatTimeJapanese(t *testing.T) {
	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{"midnight", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "0秒"},
		{"seconds only", time.Date(2024, 1, 1, 0, 0, 30, 0, time.UTC), "30秒"},
		{"minutes and seconds", time.Date(2024, 1, 1, 0, 5, 30, 0, time.UTC), "5分30秒"},
		{"hours minutes seconds", time.Date(2024, 1, 1, 14, 30, 45, 0, time.UTC), "14時30分45秒"},
		{"hours with zero minutes", time.Date(2024, 1, 1, 10, 0, 15, 0, time.UTC), "10時0分15秒"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTimeJapanese(tt.time)
			if result != tt.expected {
				t.Errorf("formatTimeJapanese() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero", 0, "0秒"},
		{"seconds", 45 * time.Second, "45秒"},
		{"minutes", 5*time.Minute + 30*time.Second, "5分30秒"},
		{"hours", 2*time.Hour + 15*time.Minute + 30*time.Second, "2時間15分30秒"},
		{"hours with zero minutes", 1*time.Hour + 30*time.Second, "1時間0分30秒"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestReplacePlaceholders(t *testing.T) {
	taskStart := time.Now().Add(-5 * time.Minute)

	tests := []struct {
		name      string
		msg       string
		taskStart time.Time
		checkFn   func(string) bool
	}{
		{
			"no placeholders",
			"タスク完了",
			taskStart,
			func(s string) bool { return s == "タスク完了" },
		},
		{
			"time placeholder",
			"{time}に完了",
			taskStart,
			func(s string) bool { return strings.HasSuffix(s, "に完了") && strings.Contains(s, "秒") },
		},
		{
			"duration placeholder",
			"処理時間: {duration}",
			taskStart,
			func(s string) bool { return strings.HasPrefix(s, "処理時間: ") && strings.Contains(s, "分") },
		},
		{
			"zero task start",
			"{duration}",
			time.Time{},
			func(s string) bool { return s == "0秒" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replacePlaceholders(tt.msg, tt.taskStart)
			if !tt.checkFn(result) {
				t.Errorf("replacePlaceholders(%q) = %q, unexpected result", tt.msg, result)
			}
		})
	}
}
