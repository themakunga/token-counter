package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PeriodUsage holds token count and cost for one time window.
type PeriodUsage struct {
	Period string // "hourly" | "daily" | "weekly" | "monthly"
	Tokens int64
	Limit  int64
	Cost   float64 // USD estimated; 0 if unavailable
	Budget float64 // USD monthly budget; 0 = not configured
}

// Provider fetches token usage for one agent.
type Provider interface {
	Name() string
	Fetch(ctx context.Context) ([]PeriodUsage, error)
}

func buildProviders(cfg *Config, creds *Credentials) []Provider {
	var out []Provider
	for _, ac := range cfg.Agents {
		if !ac.Enabled {
			continue
		}
		var apiKey string
		if creds.Agents != nil {
			if c, ok := creds.Agents[ac.ID]; ok {
				apiKey = c.APIKey
			}
			// "openai" key also accepted for "codex" id
			if apiKey == "" {
				if c, ok := creds.Agents["openai"]; ok && (ac.ID == "codex" || ac.ID == "openai") {
					apiKey = c.APIKey
				}
			}
		}
		switch ac.ID {
		case "claude":
			out = append(out, &claudeProvider{cfg: ac})
		case "openai", "codex":
			out = append(out, &openAIProvider{cfg: ac, apiKey: apiKey})
		}
		// ponytail: unknown IDs silently skipped; add a generic HTTP provider when needed
	}
	return out
}

// ─── Claude Code (local JSONL) ───────────────────────────────────────────────
//
// Data source: ~/.claude/projects/**/*.jsonl
// No credentials needed — files are local to the machine.
// Each line with type:"assistant" carries message.usage.{input,output}_tokens
// and a top-level timestamp in RFC3339.

type claudeProvider struct{ cfg AgentConfig }

func (c *claudeProvider) Name() string { return c.cfg.Name }

type claudeEntry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// claudeCost returns estimated USD cost for one assistant turn.
// ponytail: prefix-match by family; update prices when Anthropic reprices.
func claudeCost(model string, input, output, cacheCreate, cacheRead int64) float64 {
	var in, out, cc, cr float64
	switch {
	case strings.Contains(model, "opus"):
		in, out, cc, cr = 15, 75, 18.75, 1.50
	case strings.Contains(model, "haiku"):
		in, out, cc, cr = 0.80, 4, 1.00, 0.08
	default: // sonnet and unrecognised → sonnet tier
		in, out, cc, cr = 3, 15, 3.75, 0.30
	}
	return (float64(input)*in + float64(output)*out + float64(cacheCreate)*cc + float64(cacheRead)*cr) / 1_000_000
}

// fileTotals accumulates tokens and USD cost per window from one JSONL file.
type fileTotals struct {
	tokens [4]int64
	costs  [4]float64
}

func (c *claudeProvider) Fetch(_ context.Context) ([]PeriodUsage, error) {
	now := time.Now().UTC()
	cutoffs := [4]time.Time{
		now.Add(-time.Hour),           // hourly
		now.Add(-24 * time.Hour),      // daily
		now.Add(-7 * 24 * time.Hour),  // weekly
		now.Add(-30 * 24 * time.Hour), // monthly (rolling 30d)
	}
	var totals fileTotals

	root := filepath.Join(os.Getenv("HOME"), ".claude", "projects")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		ft := parseClaudeJSONL(path, cutoffs)
		for i := range totals.tokens {
			totals.tokens[i] += ft.tokens[i]
			totals.costs[i] += ft.costs[i]
		}
		return nil
	})
	if err != nil && os.IsNotExist(err) {
		return nil, fmt.Errorf("~/.claude/projects not found — is Claude Code installed?")
	}

	return []PeriodUsage{
		{Period: "hourly", Tokens: totals.tokens[0], Limit: c.cfg.Limits.Hourly, Cost: totals.costs[0]},
		{Period: "daily", Tokens: totals.tokens[1], Limit: c.cfg.Limits.Daily, Cost: totals.costs[1]},
		{Period: "weekly", Tokens: totals.tokens[2], Limit: c.cfg.Limits.Weekly, Cost: totals.costs[2]},
		{Period: "monthly", Tokens: totals.tokens[3], Limit: c.cfg.Limits.Monthly, Cost: totals.costs[3], Budget: c.cfg.MonthlyBudget},
	}, nil
}

func parseClaudeJSONL(path string, cutoffs [4]time.Time) (ft fileTotals) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 2*1024*1024), 2*1024*1024) // 2 MB — handles large context dumps

	for sc.Scan() {
		var e claudeEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Type != "assistant" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			ts, err = time.Parse("2006-01-02T15:04:05.000Z", e.Timestamp)
			if err != nil {
				continue
			}
		}
		u := e.Message.Usage
		tokens := u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
		if tokens == 0 {
			continue
		}
		cost := claudeCost(e.Message.Model, u.InputTokens, u.OutputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens)
		for i, cutoff := range cutoffs {
			if ts.After(cutoff) {
				ft.tokens[i] += tokens
				ft.costs[i] += cost
			}
		}
	}
	return
}

// ─── OpenAI / Codex ──────────────────────────────────────────────────────────
//
// Data source: https://api.openai.com/v1/organization/usage/completions
// Requires: api_key with usage.read scope (generate at platform.openai.com/api-keys).
// Set in ~/.token-counter.yaml under agents.openai.api_key.

type openAIProvider struct {
	cfg    AgentConfig
	apiKey string
}

func (o *openAIProvider) Name() string { return o.cfg.Name }

func (o *openAIProvider) Fetch(ctx context.Context) ([]PeriodUsage, error) {
	if o.apiKey == "" {
		return noKeyUsage(o.cfg), nil
	}
	now := time.Now()
	// One call per window — OpenAI usage API returns small payloads.
	results := make([]PeriodUsage, 3)
	windows := []struct {
		period string
		start  time.Time
		limit  int64
	}{
		{"hourly", now.Add(-time.Hour), o.cfg.Limits.Hourly},
		{"daily", now.Add(-24 * time.Hour), o.cfg.Limits.Daily},
		{"weekly", now.Add(-7 * 24 * time.Hour), o.cfg.Limits.Weekly},
		{"monthly", now.Add(-30 * 24 * time.Hour), o.cfg.Limits.Monthly},
	}
	for i, w := range windows {
		tokens, err := o.fetchWindow(ctx, w.start, now)
		if err != nil {
			return nil, fmt.Errorf("openai %s: %w", w.period, err)
		}
		cost := float64(tokens) * openAIRatePerM / 1_000_000
		budget := 0.0
		if w.period == "monthly" {
			budget = o.cfg.MonthlyBudget
		}
		results[i] = PeriodUsage{Period: w.period, Tokens: tokens, Limit: w.limit, Cost: cost, Budget: budget}
	}
	return results, nil
}

// noKeyUsage returns no-key sentinel rows so the table still shows the agent.
func noKeyUsage(cfg AgentConfig) []PeriodUsage {
	return []PeriodUsage{
		{Period: "hourly", Tokens: -1, Limit: cfg.Limits.Hourly},
		{Period: "daily", Tokens: -1, Limit: cfg.Limits.Daily},
		{Period: "weekly", Tokens: -1, Limit: cfg.Limits.Weekly},
		{Period: "monthly", Tokens: -1, Limit: cfg.Limits.Monthly, Budget: cfg.MonthlyBudget},
	}
}

// ponytail: blended ~$3/M for OpenAI; add per-model breakdown if accuracy matters.
const openAIRatePerM = 3.0

type openAIResp struct {
	Data []struct {
		Results []struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"results"`
	} `json:"data"`
}

func (o *openAIProvider) fetchWindow(ctx context.Context, start, end time.Time) (int64, error) {
	url := fmt.Sprintf(
		"https://api.openai.com/v1/organization/usage/completions?start_time=%d&end_time=%d&bucket_width=1d",
		start.Unix(), end.Unix(),
	)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return 0, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}

	var payload openAIResp
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, err
	}

	var total int64
	for _, bucket := range payload.Data {
		for _, r := range bucket.Results {
			total += r.InputTokens + r.OutputTokens
		}
	}
	return total, nil
}
