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
	Stats     Stats     `json:"stats"`
	Errors    []Event   `json:"errors"`
}

// Stats is the traffic attributed to one key since startup.
type Stats struct {
	Requests   int        `json:"requests"`
	Errors     int        `json:"errors"`
	Prompt     int        `json:"promptTokens"`
	Completion int        `json:"completionTokens"`
	Total      int        `json:"totalTokens"`
	LastUsed   *time.Time `json:"lastUsed,omitempty"`
}

// Event is one upstream failure attributed to a key.
type Event struct {
	Time    time.Time `json:"time"`
	Route   string    `json:"route"`
	Model   string    `json:"model"`
	Status  int       `json:"status,omitempty"`
	Message string    `json:"message"`
}

// maxKeyErrors caps the remembered error events per key.
const maxKeyErrors = 10

// keyStats mutates under the manager lock.
type keyStats struct {
	Stats
	errors []Event
}

// Manager is a concurrency-safe pool of API keys.
type Manager struct {
	mu         sync.Mutex
	keys       []Key
	next       int
	roundRobin bool
	stats      map[string]*keyStats
}

// New builds a pool from persisted keys.
func New(initial []Key, roundRobin bool) *Manager {
	return &Manager{keys: initial, roundRobin: roundRobin, stats: make(map[string]*keyStats)}
}

// Hint masks a key for display.
func Hint(secret string) string {
	if len(secret) <= 4 {
		return "••••"
	}
	return "••••" + secret[len(secret)-4:]
}

// RecordUsage attributes one relayed completion to a key.
func (m *Manager) RecordUsage(id string, prompt, completion, total int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stats := m.stats[id]
	if stats == nil {
		return
	}
	stats.Requests++
	stats.Prompt += prompt
	stats.Completion += completion
	stats.Total += total
	now := time.Now()
	stats.LastUsed = &now
}

// RecordError attributes one upstream failure to a key. A status of 0 means
// the upstream call failed before a response arrived.
func (m *Manager) RecordError(id, route, model string, status int, message string) {
	if message == "" {
		message = "upstream request failed"
	}
	if len(message) > 300 {
		message = message[:300]
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	stats := m.stats[id]
	if stats == nil {
		return
	}
	stats.Errors++
	stats.errors = append(stats.errors, Event{
		Time:    time.Now(),
		Route:   route,
		Model:   model,
		Status:  status,
		Message: message,
	})
	if len(stats.errors) > maxKeyErrors {
		stats.errors = stats.errors[len(stats.errors)-maxKeyErrors:]
	}
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
		view := View{
			ID:        key.ID,
			Label:     key.Label,
			Hint:      Hint(key.Secret),
			Enabled:   key.Enabled,
			CreatedAt: key.CreatedAt,
			Source:    key.Source,
			Errors:    []Event{},
		}
		if stats := m.stats[key.ID]; stats != nil {
			view.Stats = stats.Stats
			view.Errors = append(view.Errors, stats.errors...)
		}
		views = append(views, view)
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
func (m *Manager) Pick() (Key, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	enabled := make([]Key, 0, len(m.keys))
	for _, key := range m.keys {
		if key.Enabled {
			enabled = append(enabled, key)
		}
	}
	if len(enabled) == 0 {
		return Key{}, ErrNoKeys
	}
	selected := enabled[0]
	if m.roundRobin {
		selected = enabled[m.next%len(enabled)]
		m.next = (m.next + 1) % len(enabled)
	}
	if _, ok := m.stats[selected.ID]; !ok {
		m.stats[selected.ID] = &keyStats{}
	}
	return selected, nil
}
