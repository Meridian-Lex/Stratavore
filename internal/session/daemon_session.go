package session

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const (
	// DaemonSessionName is the fixed tmux session name for the stratavored daemon.
	DaemonSessionName = "stratavore-daemon"

	daemonSessionFileName = "daemon-session.json"
	daemonPIDFileName     = "daemon.pid"
)

// DaemonSession records the state of a running stratavored instance so it can
// be re-attached or relaunched with identical flags after a disconnect.
type DaemonSession struct {
	// SessionName is the tmux session name (empty if launched without tmux).
	SessionName string `json:"session_name,omitempty"`

	// StartedAt is when stratavored was launched.
	StartedAt time.Time `json:"started_at"`

	// PID is the stratavored process PID, written by the daemon on startup.
	PID int `json:"pid,omitempty"`

	// Flags are the CLI-level flags that were passed to 'stratavore daemon start',
	// e.g. ["--god", "--preset", "production"]. Stored so 'stratavore resume' can
	// relaunch with an identical invocation if the session is dead.
	Flags []string `json:"flags,omitempty"`

	// ConfigFile is the --config path used at launch, if any.
	ConfigFile string `json:"config_file,omitempty"`
}

// sessionDir returns the base cache directory for stratavore session files.
func sessionDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("user cache dir: %w", err)
	}
	return filepath.Join(dir, "stratavore"), nil
}

// sessionFilePath returns the full path to the daemon session record file.
func sessionFilePath() (string, error) {
	dir, err := sessionDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, daemonSessionFileName), nil
}

// pidFilePath returns the full path to the daemon PID file.
func pidFilePath() (string, error) {
	dir, err := sessionDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, daemonPIDFileName), nil
}

// Save writes the session record to disk (0600 permissions).
func (s *DaemonSession) Save() error {
	path, err := sessionFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// LoadDaemonSession reads the session record from disk.
// Returns nil, nil when no record exists.
func LoadDaemonSession() (*DaemonSession, error) {
	path, err := sessionFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session file: %w", err)
	}
	var s DaemonSession
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("corrupt session file: %w", err)
	}
	return &s, nil
}

// DeleteDaemonSession removes the session record and PID file.
func DeleteDaemonSession() error {
	sessionPath, err := sessionFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(sessionPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	pidPath, err := pidFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// WritePIDFile writes the current process PID to the daemon PID file.
// Called by stratavored on startup.
func WritePIDFile() error {
	path, err := pidFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0600)
}

// ReadPIDFile returns the PID from the daemon PID file, or 0 if not found.
func ReadPIDFile() int {
	path, err := pidFilePath()
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	return pid
}

// TmuxAvailable returns true if tmux is present in PATH.
func TmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// TmuxSessionAlive returns true if the named tmux session exists.
func TmuxSessionAlive(sessionName string) bool {
	return exec.Command("tmux", "has-session", "-t", sessionName).Run() == nil
}

// ProcessAlive returns true if a process with the given PID is running.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// Alive returns true if the daemon is still running.
// Checks the tmux session first, falls back to PID check.
func (s *DaemonSession) Alive() bool {
	if s.SessionName != "" && TmuxSessionAlive(s.SessionName) {
		return true
	}
	pid := s.PID
	if pid == 0 {
		pid = ReadPIDFile()
	}
	return ProcessAlive(pid)
}

// Attach replaces the current process with tmux attach, transferring stdio.
// Returns an error if the session name is not set or the tmux session is gone.
func (s *DaemonSession) Attach() error {
	if s.SessionName == "" {
		return fmt.Errorf("no tmux session recorded — daemon was started without tmux")
	}
	if !TmuxSessionAlive(s.SessionName) {
		return fmt.Errorf("tmux session %q is not alive (daemon may have exited)", s.SessionName)
	}
	cmd := exec.Command("tmux", "attach-session", "-t", s.SessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
