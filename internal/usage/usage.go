// Package usage records token usage observed while relaying upstream replies.
package usage

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Record is one billed upstream completion.
type Record struct {
	ID          string    `json:"id"`
	Time        time.Time `json:"time"`
	Route       string    `json:"route"`
	Model       string    `json:"model"`
	Prompt      int       `json:"promptTokens"`
	Completion  int       `json:"completionTokens"`
	Total       int       `json:"totalTokens"`
	Stream      bool      `json:"stream"`
	CachedInput int       `json:"cachedInputTokens"`
}

// Totals aggregates token counts.
type Totals struct {
	Requests   int `json:"requests"`
	Prompt     int `json:"promptTokens"`
	Completion int `json:"completionTokens"`
	Total      int `json:"totalTokens"`
}

// ModelTotals is the aggregate for one model.
type ModelTotals struct {
	Model string `json:"model"`
	Totals
}

// Snapshot is the current state served to the dashboard.
type Snapshot struct {
	Since     time.Time     `json:"since"`
	Totals    Totals        `json:"totals"`
	ByModel   []ModelTotals `json:"byModel"`
	Recent    []Record      `json:"recent"`
	LastEvent *Record       `json:"lastEvent,omitempty"`
}

// Tracker keeps the latest records in memory. It is safe for concurrent use.
type Tracker struct {
	mu      sync.Mutex
	started time.Time
	totals  Totals
	byModel map[string]*Totals
	recent  []Record
	nextID  uint64
}

// New builds an empty tracker.
func New() *Tracker {
	return &Tracker{
		started: time.Now(),
		byModel: make(map[string]*Totals),
	}
}

// Add records one completion. Records without tokens are ignored.
func (t *Tracker) Add(route, model string, prompt, completion, total, cached int, stream bool) {
	if total == 0 {
		total = prompt + completion
	}
	if prompt == 0 && completion == 0 && total == 0 {
		return
	}
	if model == "" {
		model = "unknown"
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.nextID++
	record := Record{
		ID:          fmt.Sprintf("req_%06d", t.nextID),
		Time:        time.Now(),
		Route:       route,
		Model:       model,
		Prompt:      prompt,
		Completion:  completion,
		Total:       total,
		Stream:      stream,
		CachedInput: cached,
	}

	t.totals.Requests++
	t.totals.Prompt += prompt
	t.totals.Completion += completion
	t.totals.Total += total

	modelTotals := t.byModel[model]
	if modelTotals == nil {
		modelTotals = &Totals{}
		t.byModel[model] = modelTotals
	}
	modelTotals.Requests++
	modelTotals.Prompt += prompt
	modelTotals.Completion += completion
	modelTotals.Total += total

	t.recent = append(t.recent, record)
	if len(t.recent) > 200 {
		t.recent = t.recent[len(t.recent)-200:]
	}
}

// Snapshot returns a copy of the current aggregates, newest records first.
func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot := Snapshot{
		Since:  t.started,
		Totals: t.totals,
		Recent: make([]Record, 0, len(t.recent)),
	}
	for i := len(t.recent) - 1; i >= 0; i-- {
		snapshot.Recent = append(snapshot.Recent, t.recent[i])
	}
	if len(snapshot.Recent) > 0 {
		last := snapshot.Recent[0]
		snapshot.LastEvent = &last
	}
	for model, totals := range t.byModel {
		snapshot.ByModel = append(snapshot.ByModel, ModelTotals{Model: model, Totals: *totals})
	}
	return snapshot
}

// Tokens is the usage block Mistral returns with a completion.
type Tokens struct {
	Prompt     int
	Completion int
	Total      int
	Cached     int
	OK         bool
}

// FromMessage extracts model and usage from one upstream JSON payload. It
// accepts completion responses and streaming chunks.
func FromMessage(data []byte) (string, Tokens) {
	var payload struct {
		Model string `json:"model"`
		Usage *struct {
			PromptTokens       int `json:"prompt_tokens"`
			CompletionTokens   int `json:"completion_tokens"`
			TotalTokens        int `json:"total_tokens"`
			PromptTokensCached int `json:"prompt_tokens_cached"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Usage == nil {
		return "", Tokens{}
	}
	u := payload.Usage
	return payload.Model, Tokens{
		Prompt:     u.PromptTokens,
		Completion: u.CompletionTokens,
		Total:      u.TotalTokens,
		Cached:     u.PromptTokensCached,
		OK:         true,
	}
}
