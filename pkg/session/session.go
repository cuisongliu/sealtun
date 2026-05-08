package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/labring/sealtun/pkg/auth"
)

const sessionsDirName = "sessions"
const sessionLockFileName = "sessions.lock"

const sessionLockWait = 10 * time.Second

var tunnelIDPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,53}[a-z0-9])?$`)

const (
	ConnectionStatePending    = "pending"
	ConnectionStateConnecting = "connecting"
	ConnectionStateConnected  = "connected"
	ConnectionStateError      = "error"
	ConnectionStateStopped    = "stopped"
)

type TunnelSession struct {
	TunnelID        string   `json:"tunnelId"`
	Region          string   `json:"region"`
	Namespace       string   `json:"namespace"`
	Kubeconfig      string   `json:"kubeconfig,omitempty"`
	Protocol        string   `json:"protocol"`
	Host            string   `json:"host"`
	SealosHost      string   `json:"sealosHost,omitempty"`
	CustomDomain    string   `json:"customDomain,omitempty"`
	LocalPort       string   `json:"localPort"`
	Secret          string   `json:"secret,omitempty"`
	Mode            string   `json:"mode,omitempty"`
	PID             int      `json:"pid"`
	ConnectionState string   `json:"connectionState,omitempty"`
	LastError       string   `json:"lastError,omitempty"`
	LastConnectedAt string   `json:"lastConnectedAt,omitempty"`
	UpdatedAt       string   `json:"updatedAt,omitempty"`
	CreatedAt       string   `json:"createdAt"`
	Resources       []string `json:"resources"`
}

func SessionsDir() (string, error) {
	root, err := auth.GetSealosDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(root, sessionsDirName)
	if _, err := auth.EnsurePrivateDir(dir, "sessions directory"); err != nil {
		return "", err
	}
	return dir, nil
}

func Save(session TunnelSession) error {
	release, err := acquireSessionLock()
	if err != nil {
		return err
	}
	defer release()

	return saveLocked(session)
}

func Update(session TunnelSession) error {
	release, err := acquireSessionLock()
	if err != nil {
		return err
	}
	defer release()

	if session.TunnelID == "" {
		return fmt.Errorf("session tunnel id is required")
	}
	if err := validateTunnelID(session.TunnelID); err != nil {
		return err
	}

	dir, err := SessionsDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, session.TunnelID+".json")
	if _, err := os.Stat(path); err != nil {
		return err
	}

	return saveLocked(session)
}

func saveLocked(session TunnelSession) error {
	if err := validateTunnelID(session.TunnelID); err != nil {
		return err
	}

	dir, err := SessionsDir()
	if err != nil {
		return err
	}

	if session.CreatedAt == "" {
		session.CreatedAt = time.Now().Format(time.RFC3339)
	}
	session.UpdatedAt = time.Now().Format(time.RFC3339)

	path := filepath.Join(dir, session.TunnelID+".json")
	if err := validateSessionFileForWrite(path, session.TunnelID); err != nil {
		return err
	}
	preserveScrubbedCredentials(path, &session)

	data, err := json.MarshalIndent(session, "", "  ") // #nosec G117 -- tunnel secrets are intentionally persisted with 0600 permissions for daemon reconnects.
	if err != nil {
		return err
	}

	tmpPath := filepath.Join(dir, fmt.Sprintf("%s.%d.%d.tmp", session.TunnelID, os.Getpid(), time.Now().UnixNano()))
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func validateSessionFileForWrite(path, tunnelID string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("session file %s is not a regular file", tunnelID)
	}
	return nil
}

func acquireSessionLock() (func(), error) {
	root, err := auth.GetSealosDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, sessionLockFileName)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- lock path is fixed under the user-owned Sealtun config directory.
	if err != nil {
		return nil, err
	}
	if err := lockSessionFile(file, sessionLockWait); err != nil {
		_ = file.Close()
		return nil, err
	}

	return func() {
		_ = unlockSessionFile(file)
		_ = file.Close()
	}, nil
}

func preserveScrubbedCredentials(path string, next *TunnelSession) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is derived from a validated tunnel ID under the session directory.
	if err != nil {
		return
	}

	var existing TunnelSession
	if err := json.Unmarshal(data, &existing); err != nil {
		return
	}
	if existing.TunnelID != next.TunnelID {
		return
	}
	if existing.Secret == "" {
		next.Secret = ""
		next.PID = 0
		next.ConnectionState = ConnectionStateStopped
		if next.LastError == "" {
			next.LastError = "local credentials scrubbed"
		}
	}
	if existing.Kubeconfig == "" {
		next.Kubeconfig = ""
	}
}

func Delete(tunnelID string) error {
	if err := validateTunnelID(tunnelID); err != nil {
		return err
	}

	release, err := acquireSessionLock()
	if err != nil {
		return err
	}
	defer release()

	dir, err := SessionsDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, tunnelID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ScrubCredentials() error {
	release, err := acquireSessionLock()
	if err != nil {
		return err
	}
	defer release()

	dir, err := SessionsDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !isSessionJSONFile(entry) {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path) // #nosec G304 -- entry is checked to be a regular .json file from the session directory.
		if err != nil {
			return err
		}

		var sess TunnelSession
		if err := json.Unmarshal(data, &sess); err != nil {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if err := validateTunnelID(sess.TunnelID); err != nil {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if sess.Kubeconfig == "" && sess.Secret == "" && sess.PID == 0 && sess.ConnectionState == ConnectionStateStopped {
			continue
		}

		sess.Kubeconfig = ""
		sess.Secret = ""
		sess.PID = 0
		sess.ConnectionState = ConnectionStateStopped
		sess.LastError = "local credentials scrubbed"
		if err := saveLocked(sess); err != nil {
			return fmt.Errorf("scrub session %s credentials: %w", sess.TunnelID, err)
		}
	}

	return nil
}

func Get(tunnelID string) (*TunnelSession, error) {
	if err := validateTunnelID(tunnelID); err != nil {
		return nil, err
	}

	release, err := acquireSessionLock()
	if err != nil {
		return nil, err
	}
	defer release()

	dir, err := SessionsDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, tunnelID+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("session file %s is not a regular file", tunnelID)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is derived from a validated tunnel ID under the session directory.
	if err != nil {
		return nil, err
	}

	var sess TunnelSession
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("parse session %s: %w", tunnelID, err)
	}
	if sess.TunnelID != tunnelID {
		return nil, fmt.Errorf("session file %s contains tunnel id %q", tunnelID, sess.TunnelID)
	}
	if err := validateTunnelID(sess.TunnelID); err != nil {
		return nil, err
	}
	return &sess, nil
}

func List() ([]TunnelSession, error) {
	release, err := acquireSessionLock()
	if err != nil {
		return nil, err
	}
	defer release()

	return listLocked()
}

func listLocked() ([]TunnelSession, error) {
	dir, err := SessionsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	sessions := make([]TunnelSession, 0, len(entries))
	for _, entry := range entries {
		if !isSessionJSONFile(entry) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name())) // #nosec G304 -- entry is checked to be a regular .json file from the session directory.
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		var sess TunnelSession
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		if err := validateTunnelID(sess.TunnelID); err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].TunnelID < sessions[j].TunnelID
	})

	return sessions, nil
}

func isSessionJSONFile(entry os.DirEntry) bool {
	if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
		return false
	}
	info, err := entry.Info()
	return err == nil && info.Mode().IsRegular()
}

func validateTunnelID(tunnelID string) error {
	if tunnelID == "" {
		return fmt.Errorf("session tunnel id is required")
	}
	if !tunnelIDPattern.MatchString(tunnelID) {
		return fmt.Errorf("invalid session tunnel id %q", tunnelID)
	}
	return nil
}

func IsStale(sess TunnelSession, gracePeriod time.Duration) bool {
	return IsStaleWithOwner(sess, gracePeriod, OwnerAlive(sess))
}

func IsStaleWithOwner(sess TunnelSession, gracePeriod time.Duration, ownerAlive bool) bool {
	if sess.ConnectionState == ConnectionStateStopped {
		return true
	}

	if ownerAlive {
		return false
	}

	if gracePeriod <= 0 {
		return true
	}

	ts := sess.UpdatedAt
	if ts == "" {
		ts = sess.CreatedAt
	}
	if ts == "" {
		return true
	}

	createdAt, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return true
	}

	return time.Since(createdAt) >= gracePeriod
}

func OwnerAlive(sess TunnelSession) bool {
	return ProcessAlive(sess.PID)
}

func RuntimeStatus(sess TunnelSession) string {
	return RuntimeStatusWithOwner(sess, OwnerAlive(sess))
}

func RuntimeStatusWithOwner(sess TunnelSession, ownerAlive bool) string {
	if sess.ConnectionState == ConnectionStateStopped {
		return "stopped"
	}

	if !ownerAlive {
		return "stale"
	}

	if sess.Mode != "daemon" {
		return "active"
	}

	switch sess.ConnectionState {
	case ConnectionStateConnected:
		return "active"
	case ConnectionStatePending, ConnectionStateConnecting:
		return "connecting"
	case ConnectionStateError:
		return "error"
	default:
		return "stale"
	}
}
