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
	Reasoning   int       `json:"reasoningTokens"`
	LatencyMS   int       `json:"latencyMs"`
}

// Totals aggregates token counts.
type Totals struct {
	Requests   int `json:"requests"`
	Prompt     int `json:"promptTokens"`
	Completion int `json:"completionTokens"`
	Total      int `json:"totalTokens"`
	Cached     int `json:"cachedTokens"`
	Reasoning  int `json:"reasoningTokens"`
	LatencyMS  int `json:"latencyMs"`
}

// Sample is one completion observed by the proxy.
type Sample struct {
	Route      string
	Model      string
	Prompt     int
	Completion int
	Total      int
	Cached     int
	Reasoning  int
	LatencyMS  int
	Stream     bool
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

// Add records one completion and returns the stored record. Records without
// tokens are ignored.
func (t *Tracker) Add(sample Sample) (Record, bool) {
	if sample.Total == 0 {
		sample.Total = sample.Prompt + sample.Completion
	}
	if sample.Prompt == 0 && sample.Completion == 0 && sample.Total == 0 {
		return Record{}, false
	}
	if sample.Model == "" {
		sample.Model = "unknown"
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.nextID++
	record := Record{
		ID:          fmt.Sprintf("req_%06d", t.nextID),
		Time:        time.Now(),
		Route:       sample.Route,
		Model:       sample.Model,
		Prompt:      sample.Prompt,
		Completion:  sample.Completion,
		Total:       sample.Total,
		Stream:      sample.Stream,
		CachedInput: sample.Cached,
		Reasoning:   sample.Reasoning,
		LatencyMS:   sample.LatencyMS,
	}

	t.totals.Requests++
	t.totals.Prompt += sample.Prompt
	t.totals.Completion += sample.Completion
	t.totals.Total += sample.Total
	t.totals.Cached += sample.Cached
	t.totals.Reasoning += sample.Reasoning
	t.totals.LatencyMS += sample.LatencyMS

	modelTotals := t.byModel[sample.Model]
	if modelTotals == nil {
		modelTotals = &Totals{}
		t.byModel[sample.Model] = modelTotals
	}
	modelTotals.Requests++
	modelTotals.Prompt += sample.Prompt
	modelTotals.Completion += sample.Completion
	modelTotals.Total += sample.Total
	modelTotals.Cached += sample.Cached
	modelTotals.Reasoning += sample.Reasoning
	modelTotals.LatencyMS += sample.LatencyMS

	t.recent = append(t.recent, record)
	if len(t.recent) > 200 {
		t.recent = t.recent[len(t.recent)-200:]
	}
	return record, true
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
	Reasoning  int
	OK         bool
}

// FromMessage extracts model and usage from one upstream JSON payload. It
// accepts completion responses and streaming chunks.
func FromMessage(data []byte) (string, Tokens) {
	var payload struct {
		Model string `json:"model"`
		Usage *struct {
			PromptTokens         int `json:"prompt_tokens"`
			CompletionTokens     int `json:"completion_tokens"`
			TotalTokens          int `json:"total_tokens"`
			PromptTokensCached   int `json:"prompt_tokens_cached"`
			CachedTokens         int `json:"cached_tokens"`
			NumCachedTokens      int `json:"num_cached_tokens"`
			CacheReadInputTokens int `json:"cache_read_input_tokens"`
			PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
			PromptTokensDetails  *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			InputTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Usage == nil {
		return "", Tokens{}
	}
	u := payload.Usage
	cached := u.PromptTokensCached
	for _, candidate := range []int{
		u.CachedTokens,
		u.NumCachedTokens,
		u.CacheReadInputTokens,
		u.PromptCacheHitTokens,
	} {
		if candidate > cached {
			cached = candidate
		}
	}
	if u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens > 0 {
		cached = u.PromptTokensDetails.CachedTokens
	} else if u.InputTokensDetails != nil && u.InputTokensDetails.CachedTokens > 0 {
		cached = u.InputTokensDetails.CachedTokens
	}
	reasoning := 0
	if u.CompletionTokensDetails != nil {
		reasoning = u.CompletionTokensDetails.ReasoningTokens
	}
	return payload.Model, Tokens{
		Prompt:     u.PromptTokens,
		Completion: u.CompletionTokens,
		Total:      u.TotalTokens,
		Cached:     cached,
		Reasoning:  reasoning,
		OK:         true,
	}
}
