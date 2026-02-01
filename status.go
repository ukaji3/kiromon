package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Status represents the current state of a monitored process
type Status struct {
	State         string    `json:"state"`
	Command       string    `json:"command"`
	PID           int       `json:"pid"`
	StartTime     time.Time `json:"start_time"`
	UpdatedAt     time.Time `json:"updated_at"`
	LastLines     []string  `json:"last_lines"`
	LastLine      string    `json:"last_line"`
	PromptMatched bool      `json:"prompt_matched"`
	IdleSeconds   float64   `json:"idle_seconds"`
}

// getStatusDir returns the directory for status files
func getStatusDir() string {
	// 1. XDG_RUNTIME_DIR (Linux) - auto-cleaned on reboot
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		d := filepath.Join(dir, "kiromon")
		os.MkdirAll(d, 0700)
		return d
	}

	// 2. TMPDIR/kiromon-<uid> (Mac/Linux fallback)
	uid := os.Getuid()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("kiromon-%d", uid))
	os.MkdirAll(dir, 0700)
	return dir
}

// cleanupStaleFiles removes status files older than 24 hours or with dead processes
func cleanupStaleFiles() {
	dir := getStatusDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	now := time.Now()
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Remove files older than 24 hours
		if now.Sub(info.ModTime()) > 24*time.Hour {
			os.Remove(filePath)
			continue
		}

		// Remove files for dead processes
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var status Status
		if err := json.Unmarshal(data, &status); err != nil {
			os.Remove(filePath)
			continue
		}

		// Check if process is still running
		if err := syscall.Kill(status.PID, 0); err != nil {
			os.Remove(filePath)
		}
	}
}

// getStatusFileWithPID returns status file path with PID for unique identification
func getStatusFileWithPID(name string, pid int) string {
	return filepath.Join(getStatusDir(), fmt.Sprintf("%s-%d.json", name, pid))
}

// findStatusFileByPID searches for a status file by PID
func findStatusFileByPID(pid int) (string, error) {
	dir := getStatusDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	suffix := fmt.Sprintf("-%d.json", pid)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), suffix) {
			return filepath.Join(dir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no status file found for PID %d", pid)
}

// readStatusWithLock reads status file with shared lock
func readStatusWithLock(filename string) (*Status, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Acquire shared lock (allows multiple readers)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		return nil, err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	// Read from the locked file descriptor
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}

	return &status, nil
}

// atomicWriteFile writes data to a file atomically using temp file + rename
func atomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)

	// Create temp file in same directory (for same filesystem rename)
	tmp, err := os.CreateTemp(dir, ".kiromon-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	// Cleanup on failure
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()

	// Write data
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}

	// Sync to disk
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	// Set permissions
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}

	// Atomic rename
	if err := os.Rename(tmpName, filename); err != nil {
		return err
	}

	tmpName = "" // Prevent cleanup since rename succeeded
	return nil
}
