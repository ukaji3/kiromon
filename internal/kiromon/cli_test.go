package kiromon

import (
	"testing"
)

func TestParseMonitorOptions(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected MonitorOptions
	}{
		{
			"empty args",
			[]string{},
			MonitorOptions{Interval: DefaultPollInterval},
		},
		{
			"name only",
			[]string{"kiro-cli"},
			MonitorOptions{Name: "kiro-cli", Interval: DefaultPollInterval},
		},
		{
			"daemon flag",
			[]string{"-d", "kiro-cli"},
			MonitorOptions{Name: "kiro-cli", Daemon: true, Interval: DefaultPollInterval},
		},
		{
			"with PID",
			[]string{"-p", "12345", "kiro-cli"},
			MonitorOptions{Name: "kiro-cli", PID: 12345, Interval: DefaultPollInterval},
		},
		{
			"with interval",
			[]string{"-i", "5.0", "kiro-cli"},
			MonitorOptions{Name: "kiro-cli", Interval: 5.0},
		},
		{
			"with command",
			[]string{"-c", "notify-send", "kiro-cli"},
			MonitorOptions{Name: "kiro-cli", Command: "notify-send", Interval: DefaultPollInterval},
		},
		{
			"with messages",
			[]string{"-ms", "start", "-me", "end", "kiro-cli"},
			MonitorOptions{Name: "kiro-cli", StartMsg: "start", EndMsg: "end", Interval: DefaultPollInterval},
		},
		{
			"with prompt pattern",
			[]string{"-r", "> ?$", "kiro-cli"},
			MonitorOptions{Name: "kiro-cli", PromptPattern: "> ?$", Interval: DefaultPollInterval},
		},
		{
			"full options",
			[]string{"-d", "-p", "999", "-i", "3.0", "-c", "cmd", "-ms", "s", "-me", "e", "-r", "pat", "name"},
			MonitorOptions{
				Name:          "name",
				PID:           999,
				Daemon:        true,
				Interval:      3.0,
				Command:       "cmd",
				StartMsg:      "s",
				EndMsg:        "e",
				PromptPattern: "pat",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMonitorOptions(tt.args)

			if result.Name != tt.expected.Name {
				t.Errorf("Name = %q, want %q", result.Name, tt.expected.Name)
			}
			if result.PID != tt.expected.PID {
				t.Errorf("PID = %d, want %d", result.PID, tt.expected.PID)
			}
			if result.Daemon != tt.expected.Daemon {
				t.Errorf("Daemon = %v, want %v", result.Daemon, tt.expected.Daemon)
			}
			if result.Interval != tt.expected.Interval {
				t.Errorf("Interval = %f, want %f", result.Interval, tt.expected.Interval)
			}
			if result.Command != tt.expected.Command {
				t.Errorf("Command = %q, want %q", result.Command, tt.expected.Command)
			}
			if result.StartMsg != tt.expected.StartMsg {
				t.Errorf("StartMsg = %q, want %q", result.StartMsg, tt.expected.StartMsg)
			}
			if result.EndMsg != tt.expected.EndMsg {
				t.Errorf("EndMsg = %q, want %q", result.EndMsg, tt.expected.EndMsg)
			}
			if result.PromptPattern != tt.expected.PromptPattern {
				t.Errorf("PromptPattern = %q, want %q", result.PromptPattern, tt.expected.PromptPattern)
			}
		})
	}
}
