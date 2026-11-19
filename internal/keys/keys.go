// Package keys holds the upstream credential pool and round-robin selection.
package keys

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ErrNoKeys means every configured credential is disabled or the pool is empty.
var ErrNoKeys = errors.New("no enabled API keys")

// Key is one upstream credential. Secret is never exposed by JSON views.
type Key struct {
	ID        string
	Label     string
	Secret    string
	Enabled   bool
	CreatedAt time.Time
	Source    string // "database", "environment", or "memory"
}

// View is the credential as shown in the dashboard.
type View struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Hint      string    `json:"hint"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	Source    string    `json:"source"`
}

// Manager is a concurrency-safe pool of API keys.
type Manager struct {
	mu         sync.Mutex
	keys       []Key
	next       int
	roundRobin bool
}

// New builds a pool from persisted keys.
func New(initial []Key, roundRobin bool) *Manager {
	return &Manager{keys: initial, roundRobin: roundRobin}
}

// Hint masks a key for display.
func Hint(secret string) string {
	if len(secret) <= 4 {
		return "••••"
	}
	return "••••" + secret[len(secret)-4:]
}

// NewID returns an id for an in-memory key.
func NewID() string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "mem_" + time.Now().Format("20060102150405")
	}
	return "mem_" + hex.EncodeToString(raw[:])
}

// Add appends a key.
func (m *Manager) Add(key Key) {
	m.mu.Lock()
	m.keys = append(m.keys, key)
	m.mu.Unlock()
}

// ContainsSecret reports whether the exact key is already configured.
func (m *Manager) ContainsSecret(secret string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range m.keys {
		if key.Secret == secret {
			return true
		}
	}
	return false
}

// Get returns one key by id.
func (m *Manager) Get(id string) (Key, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range m.keys {
		if key.ID == id {
			return key, true
		}
	}
	return Key{}, false
}

// Update changes a label and/or enabled state.
func (m *Manager) Update(id string, label *string, enabled *bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.keys {
		if m.keys[i].ID != id {
			continue
		}
		if label != nil {
			m.keys[i].Label = *label
		}
		if enabled != nil {
			m.keys[i].Enabled = *enabled
		}
		return true
	}
	return false
}

// Delete removes one key.
func (m *Manager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.keys {
		if m.keys[i].ID == id {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			if m.next > len(m.keys) {
				m.next = 0
			}
			return true
		}
	}
	return false
}

// Views returns every key in insertion order, without secrets.
func (m *Manager) Views() []View {
	m.mu.Lock()
	defer m.mu.Unlock()
	views := make([]View, 0, len(m.keys))
	for _, key := range m.keys {
		views = append(views, View{
			ID:        key.ID,
			Label:     key.Label,
			Hint:      Hint(key.Secret),
			Enabled:   key.Enabled,
			CreatedAt: key.CreatedAt,
			Source:    key.Source,
		})
	}
	return views
}

// Count returns the number of configured keys.
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.keys)
}

// EnabledCount returns the number of enabled keys.
func (m *Manager) EnabledCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, key := range m.keys {
		if key.Enabled {
			count++
		}
	}
	return count
}

// FirstHint returns the hint of the first enabled key.
func (m *Manager) FirstHint() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range m.keys {
		if key.Enabled {
			return Hint(key.Secret)
		}
	}
	return ""
}

// SetRoundRobin enables or disables rotation.
func (m *Manager) SetRoundRobin(enabled bool) {
	m.mu.Lock()
	m.roundRobin = enabled
	m.mu.Unlock()
}

// RoundRobin reports whether rotation is enabled.
func (m *Manager) RoundRobin() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.roundRobin
}

// Pick returns the next credential. Rotation only advances when round-robin
// is enabled; otherwise every request uses the first enabled key.
func (m *Manager) Pick() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	enabled := make([]string, 0, len(m.keys))
	for _, key := range m.keys {
		if key.Enabled {
			enabled = append(enabled, key.Secret)
		}
	}
	if len(enabled) == 0 {
		return "", ErrNoKeys
	}
	if !m.roundRobin {
		return enabled[0], nil
	}
	selected := enabled[m.next%len(enabled)]
	m.next = (m.next + 1) % len(enabled)
	return selected, nil
}
