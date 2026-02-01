package kiromon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAtomicWriteFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("write and read", func(t *testing.T) {
		filename := filepath.Join(tmpDir, "test.json")
		data := []byte(`{"test": "data"}`)

		err := atomicWriteFile(filename, data, 0644)
		if err != nil {
			t.Fatalf("atomicWriteFile() error = %v", err)
		}

		// Verify file exists and has correct content
		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if string(content) != string(data) {
			t.Errorf("content = %q, want %q", content, data)
		}

		// Verify permissions
		info, _ := os.Stat(filename)
		if info.Mode().Perm() != 0644 {
			t.Errorf("permissions = %o, want 0644", info.Mode().Perm())
		}
	})

	t.Run("overwrite existing", func(t *testing.T) {
		filename := filepath.Join(tmpDir, "overwrite.json")

		// Write initial content
		atomicWriteFile(filename, []byte("old"), 0644)

		// Overwrite
		err := atomicWriteFile(filename, []byte("new"), 0644)
		if err != nil {
			t.Fatalf("atomicWriteFile() error = %v", err)
		}

		content, _ := os.ReadFile(filename)
		if string(content) != "new" {
			t.Errorf("content = %q, want %q", content, "new")
		}
	})
}

func TestReadStatusWithLock(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("read valid status", func(t *testing.T) {
		filename := filepath.Join(tmpDir, "status.json")
		statusJSON := `{
			"state": "waiting",
			"command": "test-cmd",
			"pid": 12345,
			"last_line": "> ",
			"prompt_matched": true,
			"idle_seconds": 1.5
		}`
		os.WriteFile(filename, []byte(statusJSON), 0644)

		status, err := readStatusWithLock(filename)
		if err != nil {
			t.Fatalf("readStatusWithLock() error = %v", err)
		}

		if status.State != "waiting" {
			t.Errorf("State = %q, want %q", status.State, "waiting")
		}
		if status.PID != 12345 {
			t.Errorf("PID = %d, want %d", status.PID, 12345)
		}
		if !status.PromptMatched {
			t.Error("PromptMatched = false, want true")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := readStatusWithLock(filepath.Join(tmpDir, "nonexistent.json"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		filename := filepath.Join(tmpDir, "invalid.json")
		os.WriteFile(filename, []byte("not json"), 0644)

		_, err := readStatusWithLock(filename)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestStatusRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "roundtrip.json")

	original := Status{
		State:         StateWaiting,
		Command:       "kiro-cli chat",
		PID:           99999,
		StartTime:     time.Now().Truncate(time.Second),
		UpdatedAt:     time.Now().Truncate(time.Second),
		LastLines:     []string{"line1", "line2"},
		LastLine:      "> ",
		PromptMatched: true,
		IdleSeconds:   2.5,
	}

	// Write using JSON marshal
	data, _ := json.MarshalIndent(original, "", "  ")
	err := atomicWriteFile(filename, data, 0600)
	if err != nil {
		t.Fatalf("atomicWriteFile() error = %v", err)
	}

	// Read back
	status, err := readStatusWithLock(filename)
	if err != nil {
		t.Fatalf("readStatusWithLock() error = %v", err)
	}

	if status.State != original.State {
		t.Errorf("State = %q, want %q", status.State, original.State)
	}
	if status.PID != original.PID {
		t.Errorf("PID = %d, want %d", status.PID, original.PID)
	}
	if status.Command != original.Command {
		t.Errorf("Command = %q, want %q", status.Command, original.Command)
	}
}
