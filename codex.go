package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func codexHome() string {
	if path := os.Getenv("CODEX_HOME"); path != "" {
		return path
	}
	return filepath.Join(os.Getenv("HOME"), ".codex")
}

type codexProvider struct{ cfg AgentConfig }

func (c *codexProvider) Name() string { return c.cfg.Name }

func (c *codexProvider) Fetch(ctx context.Context) ([]PeriodUsage, error) {
	now := time.Now()
	cutoffs := [4]time.Time{now.Add(-time.Hour), now.Add(-24 * time.Hour), now.Add(-7 * 24 * time.Hour), now.Add(-30 * 24 * time.Hour)}
	var totals [4]int64
	err := filepath.WalkDir(filepath.Join(codexHome(), "sessions"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		counts, err := parseCodexJSONL(ctx, path, cutoffs)
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		for i := range totals {
			totals[i] += counts[i]
		}
		return nil
	})
	limits := [4]int64{c.cfg.Limits.Hourly, c.cfg.Limits.Daily, c.cfg.Limits.Weekly, c.cfg.Limits.Monthly}
	periods := [4]string{"hourly", "daily", "weekly", "monthly"}
	rows := make([]PeriodUsage, 4)
	for i := range rows {
		tokens := totals[i]
		if err != nil {
			tokens = -1
		}
		rows[i] = PeriodUsage{Period: periods[i], Tokens: tokens, Limit: limits[i], CostUnavailable: true}
	}
	rows[3].Budget = c.cfg.MonthlyBudget
	return rows, err
}

func parseCodexJSONL(ctx context.Context, path string, cutoffs [4]time.Time) (counts [4]int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return counts, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	// Read only usage events, but transcripts can contain large tool results.
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var previous int64
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return counts, err
		}
		var entry struct {
			Type      string    `json:"type"`
			Timestamp time.Time `json:"timestamp"`
			Payload   struct {
				Type string `json:"type"`
				Info *struct {
					Total *struct {
						Tokens int64 `json:"total_tokens"`
					} `json:"total_token_usage"`
				} `json:"info"`
			} `json:"payload"`
		}
		// A live file may end with an incomplete JSON line. Ignore it until refresh.
		if json.Unmarshal(sc.Bytes(), &entry) != nil || entry.Type != "event_msg" || entry.Payload.Type != "token_count" || entry.Payload.Info == nil || entry.Payload.Info.Total == nil {
			continue
		}
		total := entry.Payload.Info.Total.Tokens
		if total < 0 {
			return counts, fmt.Errorf("negative token usage")
		}
		// Repeated snapshots do not consume tokens. A reset starts a new counter.
		delta := total - previous
		if total < previous {
			delta = total
		}
		previous = total
		if entry.Timestamp.IsZero() {
			continue
		}
		for i, cutoff := range cutoffs {
			if entry.Timestamp.After(cutoff) {
				counts[i] += delta
			}
		}
	}
	return counts, sc.Err()
}
