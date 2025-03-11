package manager

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// ChangeNotification represents a file system change
type ChangeNotification struct {
	Path      string
	Operation string // "create", "write", "delete", etc.
	Time      time.Time
}

type Manager struct {
	mu             sync.Mutex
	repository     *git.Repository
	authFile       string
	changes        chan ChangeNotification
	isDirty        bool
	lastChangeTime time.Time
	syncInterval   time.Duration
}

func NewManager(repository *git.Repository, authFile string) *Manager {
	return &Manager{
		repository:   repository,
		authFile:     authFile,
		changes:      make(chan ChangeNotification, 100), // Buffer size of 100
		isDirty:      false,
		syncInterval: 2 * time.Second, // Default 5 second interval
	}
}

// NotifyChange sends a notification about a filesystem change
func (m *Manager) NotifyChange(path, operation string) {
	notification := ChangeNotification{
		Path:      path,
		Operation: operation,
		Time:      time.Now(),
	}

	log.Printf("NotifyChange: %s (%s)", path, operation)

	// Send to channel without blocking if possible
	select {
	case m.changes <- notification:
		// Successfully sent
	default:
		// Channel buffer is full, log this but don't block
		fmt.Printf("Warning: change notification buffer is full, dropping change for %s\n", path)
	}
}

// SyncToGit performs the actual git operations
func (m *Manager) SyncToGit() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isDirty {
		return nil // Nothing to do
	}

	fmt.Printf("Syncing changes to git...\n")

	// Get the worktree
	wt, err := m.repository.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add all changes
	_, err = wt.Add(".")
	if err != nil {
		return fmt.Errorf("failed to add changes: %w", err)
	}

	// Commit changes
	_, err = wt.Commit("Auto-commit from gittyfs", &git.CommitOptions{})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	var authMethod transport.AuthMethod
	if m.authFile != "" {
		authMethod, err = ssh.NewPublicKeysFromFile("git", m.authFile, "")
		if err != nil {
			log.Fatalf("new public keys from file %s: %s", m.authFile, err)
		}
	} else {
		authMethod, err = ssh.NewSSHAgentAuth("git")
		if err != nil {
			log.Fatalf("new ssh agent auth %s: %s", "git", err)
		}
	}

	err = m.repository.Push(&git.PushOptions{
		Auth: authMethod,
	})
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	// Reset dirty flag
	m.isDirty = false
	fmt.Printf("Changes committed to git\n")

	return nil
}

func (m *Manager) Run() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case change := <-m.changes:
			// Process the change notification
			m.mu.Lock()
			m.isDirty = true
			m.lastChangeTime = change.Time
			fmt.Printf("Change detected: %s (%s)\n", change.Path, change.Operation)
			m.mu.Unlock()

		case <-ticker.C:
			// Check if it's time to sync
			m.mu.Lock()
			if m.isDirty && time.Since(m.lastChangeTime) >= m.syncInterval {
				// Unlock before syncing as SyncToGit will acquire the lock
				m.mu.Unlock()
				if err := m.SyncToGit(); err != nil {
					fmt.Printf("Error syncing to git: %v\n", err)
				}
			} else {
				m.mu.Unlock()
			}
		}
	}
}
