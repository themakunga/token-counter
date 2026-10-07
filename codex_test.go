package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexUsageAndLimits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	root := filepath.Join(home, "sessions", "2026")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	event := func(age time.Duration, total int64) string {
		return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":%d},"last_token_usage":{"total_tokens":99999}}}}`+"\n", now.Add(-age).Format(time.RFC3339Nano), total)
	}
	// Old snapshots establish the baseline; repeated snapshots and status-only
	// events must not count last_token_usage again. Counter resets are supported.
	data := event(40*24*time.Hour, 100) + event(8*24*time.Hour, 200) + event(2*24*time.Hour, 300) + event(2*time.Hour, 400) + event(10*time.Minute, 450) + event(9*time.Minute, 450) + `{"type":"event_msg","payload":{"type":"token_count","info":null}}` + "\n" + event(8*time.Minute, 20) + `{"partial":`
	if err := os.WriteFile(filepath.Join(root, "rollout.jsonl"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := AgentConfig{ID: "openai", Name: "Codex", Enabled: true, Limits: Limits{Hourly: 50, Daily: 500, Weekly: 2000, Monthly: 8000}, MonthlyBudget: 30}
	providers := buildProviders(&Config{Agents: []AgentConfig{cfg}}, &Credentials{})
	if _, ok := providers[0].(*codexProvider); !ok {
		t.Fatal("Existing openai config must select local Codex")
	}
	rows, err := providers[0].Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantTokens := [4]int64{70, 170, 270, 370}
	wantLimits := [4]int64{50, 500, 2000, 8000}
	for i, row := range rows {
		if row.Tokens != wantTokens[i] || row.Limit != wantLimits[i] || !row.CostUnavailable {
			t.Fatalf("row %d: %+v", i, row)
		}
	}
	if rows[3].Budget != 30 {
		t.Fatal("Budget lost")
	}
	m := newModel(&Config{Agents: []AgentConfig{cfg}}, &Credentials{})
	display := m.doFetch()().(dataMsg)
	m.rows, m.loading = display, false
	view := m.View()
	if !strings.Contains(view, "70 / 50") || !strings.Contains(view, "USD unavailable") || !strings.Contains(view, "$30.00 budget") || strings.Contains(view, "$0.00") {
		t.Fatal(view)
	}
	m.width = 80
	if view := m.View(); !strings.Contains(strings.Join(strings.Fields(view), " "), "Monthly 370 / 8.0K") {
		t.Fatal(view)
	}
	cfg.Source = "api"
	if _, ok := buildProviders(&Config{Agents: []AgentConfig{cfg}}, &Credentials{})[0].(*openAIProvider); !ok {
		t.Fatal("Explicit API source ignored")
	}
	t.Setenv("CODEX_HOME", filepath.Join(home, "missing"))
	rows, err = (&codexProvider{cfg: cfg}).Fetch(context.Background())
	if err == nil || rows[0].Tokens != -1 || rows[0].Limit != 50 {
		t.Fatalf("Missing logs should preserve limits and report error: %+v %v", rows, err)
	}
}

func TestClaudeCostAggregation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".claude", "projects")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	data := fmt.Sprintf(`{"type":"assistant","requestId":"request-1","timestamp":%q,"message":{"id":"message-1","model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":10,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"cache_creation":{"ephemeral_1h_input_tokens":200}}}}`, now.Format(time.RFC3339Nano))
	if err := os.WriteFile(filepath.Join(root, "test.jsonl"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	// Same API message copied to a second log with a later output snapshot.
	duplicate := strings.Replace(data, `"output_tokens":10`, `"output_tokens":20`, 1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	old := fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"id":"old-message","model":"claude-sonnet-4-6","usage":{"input_tokens":100}}}`, monthStart.Add(-time.Hour).Format(time.RFC3339Nano))
	if err := os.WriteFile(filepath.Join(root, "copy.jsonl"), []byte(duplicate+"\n"+old), 0600); err != nil {
		t.Fatal(err)
	}
	rows, err := fetchFromJSONL(AgentConfig{MonthlyBudget: 300})
	if err != nil || rows[3].Tokens != 1420 || math.Abs(rows[3].Cost-0.0021) > 1e-9 || math.Abs(rows[3].NetCost-0.0018) > 1e-9 || rows[3].Budget != 300 {
		t.Fatalf("Deduplication, calendar month or cache pricing broken: %+v %v", rows, err)
	}
	if math.Abs(claudeCost("claude-opus-5-5", 1000000, 1000000, 1000000, 1000000)-29.2) > 1e-9 {
		t.Fatal("Opus 5.5 pricing incorrect")
	}
}
