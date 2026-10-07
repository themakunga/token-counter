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
	Period          string // "hourly" | "daily" | "weekly" | "monthly"
	Tokens          int64
	Limit           int64
	Cost            float64 // USD total (includes cache_read)
	NetCost         float64 // USD without cache_read — new tokens only
	Budget          float64 // USD monthly budget; 0 = not configured
	CostUnavailable bool    // local Codex logs do not report billed USD
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
		case "vertex":
			var gatewayURL, projectID string
			if creds.Agents != nil {
				if c, ok := creds.Agents["vertex"]; ok {
					gatewayURL = c.GatewayURL
					projectID = c.ProjectID
				}
			}
			if gatewayURL == "" {
				gatewayURL = "http://localhost:8150" // Cosmos gateway default
			}
			out = append(out, &vertexProvider{cfg: ac, gatewayURL: gatewayURL, projectID: projectID})
		case "openai", "codex":
			_, localErr := os.Stat(filepath.Join(codexHome(), "sessions"))
			if ac.Source == "codex" || (ac.Source != "api" && (ac.ID == "codex" || localErr == nil)) {
				out = append(out, &codexProvider{cfg: ac})
			} else {
				out = append(out, &openAIProvider{cfg: ac, apiKey: apiKey})
			}
		}
		// ponytail: unknown IDs silently skipped; add a generic HTTP provider when needed
	}
	return out
}

// ─── Shared JSONL reader ─────────────────────────────────────────────────────
//
// Claude Code writes ~/.claude/projects/**/*.jsonl for every session,
// regardless of whether it routes to Anthropic API directly or via Vertex.
// Both claudeProvider and vertexProvider read from here.

type claudeEntry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	RequestID string `json:"requestId"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreation            struct {
				OneHour int64 `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
		} `json:"usage"`
	} `json:"message"`
}

// claudeCost returns estimated USD cost for one assistant turn.
// ponytail: prefix-match by family; update prices when Anthropic reprices.
func claudeCost(model string, input, output, cacheCreate, cacheRead int64) float64 {
	var in, out, cc, cr float64
	switch {
	case strings.Contains(model, "opus-5-5"):
		in, out, cc, cr = 4, 20, 5, 0.20
	case strings.Contains(model, "opus-4-5") || strings.Contains(model, "opus-4-6") || strings.Contains(model, "opus-4-7") || strings.Contains(model, "opus-4-8") || strings.Contains(model, "opus-5"):
		in, out, cc, cr = 5, 25, 6.25, 0.50
	case strings.Contains(model, "opus"):
		in, out, cc, cr = 15, 75, 18.75, 1.50
	case strings.Contains(model, "haiku"):
		in, out, cc, cr = 0.80, 4, 1.00, 0.08
	case strings.Contains(model, "sonnet-5"):
		in, out, cc, cr = 2, 10, 2.50, 0.20
	default: // sonnet and unrecognised → sonnet tier
		in, out, cc, cr = 3, 15, 3.75, 0.30
	}
	return (float64(input)*in + float64(output)*out + float64(cacheCreate)*cc + float64(cacheRead)*cr) / 1_000_000
}

// fileTotals accumulates tokens and USD cost per window from one JSONL file.
type fileTotals struct {
	tokens   [4]int64
	costs    [4]float64 // total cost including cache_read
	netCosts [4]float64 // cost without cache_read (new tokens only)
}

// fetchFromJSONL scans ~/.claude/projects/**/*.jsonl and returns usage for all
// four windows. Shared by claudeProvider and vertexProvider.
func fetchFromJSONL(cfg AgentConfig) ([]PeriodUsage, error) {
	now := time.Now()
	cutoffs := [4]time.Time{
		now.Add(-time.Hour),           // hourly
		now.Add(-24 * time.Hour),      // daily
		now.Add(-7 * 24 * time.Hour),  // weekly
		now.Add(-30 * 24 * time.Hour), // monthly (rolling 30d)
	}
	var totals fileTotals
	entries := make(map[string]claudeEntry)

	root := filepath.Join(os.Getenv("HOME"), ".claude", "projects")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		return readClaudeJSONL(path, entries)
	})
	if err != nil {
		return noKeyUsage(cfg), fmt.Errorf("Claude usage: %w", err)
	}
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	for _, e := range entries {
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		u := e.Message.Usage
		tokens := u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
		cost := claudeCost(e.Message.Model, u.InputTokens, u.OutputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens)
		netCost := claudeCost(e.Message.Model, u.InputTokens, u.OutputTokens, u.CacheCreationInputTokens, 0)
		// One-hour cache writes cost 2x base input instead of the 5m rate's 1.25x.
		premium := claudeCost(e.Message.Model, 0, 0, u.CacheCreation.OneHour, 0) * 0.6
		cost += premium
		netCost += premium
		for i, cutoff := range cutoffs {
			if ts.After(cutoff) {
				totals.tokens[i] += tokens
			}
			costCutoff := cutoff
			if i == 3 {
				costCutoff = monthStart
			}
			if !ts.Before(costCutoff) {
				totals.costs[i] += cost
				totals.netCosts[i] += netCost
			}
		}
	}

	return []PeriodUsage{
		{Period: "hourly", Tokens: totals.tokens[0], Limit: cfg.Limits.Hourly, Cost: totals.costs[0], NetCost: totals.netCosts[0]},
		{Period: "daily", Tokens: totals.tokens[1], Limit: cfg.Limits.Daily, Cost: totals.costs[1], NetCost: totals.netCosts[1]},
		{Period: "weekly", Tokens: totals.tokens[2], Limit: cfg.Limits.Weekly, Cost: totals.costs[2], NetCost: totals.netCosts[2]},
		{Period: "monthly", Tokens: totals.tokens[3], Limit: cfg.Limits.Monthly, Cost: totals.costs[3], NetCost: totals.netCosts[3], Budget: cfg.MonthlyBudget},
	}, nil
}

func readClaudeJSONL(path string, entries map[string]claudeEntry) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 2*1024*1024), 2*1024*1024) // 2 MB — handles large context dumps

	line := 0
	for sc.Scan() {
		line++
		var e claudeEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Type != "assistant" {
			continue
		}
		key := e.RequestID + ":" + e.Message.ID
		if e.Message.ID == "" {
			key = fmt.Sprintf("%s:%d", path, line)
		}
		// Streaming content blocks repeat input/cache usage. Keep one request's
		// maximum counters, including its final output count, across log copies.
		if old, ok := entries[key]; ok {
			u, previous := &e.Message.Usage, old.Message.Usage
			u.InputTokens = max(u.InputTokens, previous.InputTokens)
			u.OutputTokens = max(u.OutputTokens, previous.OutputTokens)
			u.CacheCreationInputTokens = max(u.CacheCreationInputTokens, previous.CacheCreationInputTokens)
			u.CacheReadInputTokens = max(u.CacheReadInputTokens, previous.CacheReadInputTokens)
			u.CacheCreation.OneHour = max(u.CacheCreation.OneHour, previous.CacheCreation.OneHour)
			if e.Timestamp < old.Timestamp {
				e.Timestamp = old.Timestamp
			}
		}
		entries[key] = e
	}
	return sc.Err()
}

// ─── Claude Code (direct API) ────────────────────────────────────────────────

type claudeProvider struct{ cfg AgentConfig }

func (c *claudeProvider) Name() string { return c.cfg.Name }

func (c *claudeProvider) Fetch(_ context.Context) ([]PeriodUsage, error) {
	return fetchFromJSONL(c.cfg)
}

// ─── Claude Code via Vertex AI (Cosmos gateway) ──────────────────────────────
//
// Data source: same ~/.claude/projects/**/*.jsonl as claudeProvider.
// Claude Code writes usage locally regardless of routing (direct or Vertex).
//
// Credentials (~/.token-counter.yaml) — never commit these values:
//   agents:
//     vertex:
//       gateway_url: "http://localhost:8150"   # local Cosmos proxy
//       project_id:  "<your-gcp-project-id>"  # SENSITIVE
//
// ponytail: JSONL is the authoritative source. Extend Fetch() to query
// the gateway's /metrics or /usage endpoint when one is documented.

type vertexProvider struct {
	cfg        AgentConfig
	gatewayURL string // e.g. http://localhost:8150 — not sensitive (localhost)
	projectID  string // GCP project ID — SENSITIVE, credentials only
}

func (v *vertexProvider) Name() string { return v.cfg.Name }

func (v *vertexProvider) Fetch(_ context.Context) ([]PeriodUsage, error) {
	return fetchFromJSONL(v.cfg)
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
	results := make([]PeriodUsage, len(windows))
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
		results[i] = PeriodUsage{Period: w.period, Tokens: tokens, Limit: w.limit, Cost: cost, NetCost: cost, Budget: budget}
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
