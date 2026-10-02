package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// version is set at build time via -ldflags="-X main.version=vX.Y.Z"
var version = "dev"

// ─── Styles ───────────────────────────────────────────────────────────────────

var (
	sGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87"))
	sYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700"))
	sRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F87"))
	sDim    = lipgloss.NewStyle().Faint(true)
	sBold   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	sBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1)
)

// ─── Cell ────────────────────────────────────────────────────────────────────

type cell struct {
	tokens int64 // -1 = no API key configured
	limit  int64
}

func (c cell) pct() float64 {
	if c.limit == 0 || c.tokens < 0 {
		return 0
	}
	return float64(c.tokens) / float64(c.limit)
}

func (c cell) render() string {
	if c.tokens < 0 {
		return sDim.Render("— no key —")
	}
	s := fmtTokens(c.tokens)
	if c.limit > 0 {
		s += " / " + fmtTokens(c.limit)
	}
	switch pct := c.pct(); {
	case pct > 0.8:
		return sRed.Render(s)
	case pct > 0.5:
		return sYellow.Render(s)
	default:
		return sGreen.Render(s)
	}
}

func fmtTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// ─── Display row ─────────────────────────────────────────────────────────────

type dRow struct {
	name          string
	hourly        cell
	daily         cell
	weekly        cell
	monthly       cell
	monthlyCost   float64 // USD
	monthlyBudget float64 // USD; 0 = not configured
}

func statusDot(r dRow) string {
	pct := maxPct(r.hourly.pct(), r.daily.pct(), r.weekly.pct(), r.monthly.pct())
	switch {
	case pct > 0.8:
		return sRed.Render("●")
	case pct > 0.5:
		return sYellow.Render("●")
	default:
		return sGreen.Render("●")
	}
}

func costStyle(spent, budget float64) lipgloss.Style {
	if budget <= 0 {
		return sGreen
	}
	pct := spent / budget
	switch {
	case pct > 0.8:
		return sRed
	case pct > 0.5:
		return sYellow
	default:
		return sGreen
	}
}

func maxPct(vals ...float64) float64 {
	var m float64
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}

// ─── Messages ────────────────────────────────────────────────────────────────

type tickMsg struct{}
type dataMsg []dRow

// ─── Model ───────────────────────────────────────────────────────────────────

type model struct {
	rows      []dRow
	cfg       *Config
	providers []Provider
	lastFetch time.Time
	loading   bool
	fetchErr  string
}

func newModel(cfg *Config, creds *Credentials) model {
	return model{
		cfg:       cfg,
		providers: buildProviders(cfg, creds),
		loading:   true,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.doFetch(), m.tick())
}

func (m model) doFetch() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		var rows []dRow
		for _, p := range m.providers {
			r := dRow{name: p.Name()}
			usages, err := p.Fetch(ctx)
			if err != nil {
				r.name += " ⚠"
			}
			for _, u := range usages {
				c := cell{tokens: u.Tokens, limit: u.Limit}
				switch u.Period {
				case "hourly":
					r.hourly = c
				case "daily":
					r.daily = c
				case "weekly":
					r.weekly = c
				case "monthly":
					r.monthly = c
					r.monthlyCost = u.Cost
					r.monthlyBudget = u.Budget
				}
			}
			rows = append(rows, r)
		}
		return dataMsg(rows)
	}
}

func (m model) tick() tea.Cmd {
	return tea.Tick(m.cfg.RefreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.loading = true
			return m, m.doFetch()
		}
	case tickMsg:
		m.loading = true
		return m, tea.Batch(m.doFetch(), m.tick())
	case dataMsg:
		m.loading = false
		m.rows = []dRow(msg)
		m.lastFetch = time.Now()
		m.fetchErr = ""
	}
	return m, nil
}

// ─── View ────────────────────────────────────────────────────────────────────

// padTo pads s to visual width w, ANSI-aware.
func padTo(s string, w int) string {
	vis := lipgloss.Width(s)
	if vis >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vis)
}

const (
	wAgent  = 18
	wPeriod = 24
)

func (m model) View() string {
	sep := "  "
	var b strings.Builder

	// Header
	b.WriteString(padTo(sBold.Render("Agent"), wAgent) + sep)
	b.WriteString(padTo(sBold.Render("Hourly"), wPeriod) + sep)
	b.WriteString(padTo(sBold.Render("Daily"), wPeriod) + sep)
	b.WriteString(padTo(sBold.Render("Weekly"), wPeriod) + sep)
	b.WriteString(padTo(sBold.Render("Monthly"), wPeriod) + sep)
	b.WriteString(sBold.Render("⬤"))
	b.WriteByte('\n')

	// Divider
	divWidth := wAgent + wPeriod*4 + len(sep)*5 + 1
	b.WriteString(sDim.Render(strings.Repeat("─", divWidth)))
	b.WriteByte('\n')

	// Rows
	if m.loading && len(m.rows) == 0 {
		b.WriteString(sDim.Render("  fetching…"))
		b.WriteByte('\n')
	} else {
		for _, r := range m.rows {
			b.WriteString(padTo(r.name, wAgent) + sep)
			b.WriteString(padTo(r.hourly.render(), wPeriod) + sep)
			b.WriteString(padTo(r.daily.render(), wPeriod) + sep)
			b.WriteString(padTo(r.weekly.render(), wPeriod) + sep)
			b.WriteString(padTo(r.monthly.render(), wPeriod) + sep)
			b.WriteString(statusDot(r))
			b.WriteByte('\n')
		}
	}

	// Monthly cost section
	var hasCost bool
	for _, r := range m.rows {
		if r.monthlyCost > 0 {
			hasCost = true
			break
		}
	}
	if hasCost && !m.loading {
		b.WriteString(sDim.Render(strings.Repeat("─", divWidth)))
		b.WriteByte('\n')
		b.WriteString(sBold.Render("Monthly spend") + "\n")
		for _, r := range m.rows {
			b.WriteString(padTo("  "+r.name, wAgent+2) + "  ")
			spent := fmt.Sprintf("$%.2f", r.monthlyCost)
			b.WriteString(costStyle(r.monthlyCost, r.monthlyBudget).Render(spent))
			if r.monthlyBudget > 0 {
				avail := r.monthlyBudget - r.monthlyCost
				b.WriteString(sDim.Render(fmt.Sprintf("  / $%.2f budget  ($%.2f avail)", r.monthlyBudget, avail)))
			}
			b.WriteByte('\n')
		}
	}

	b.WriteByte('\n')

	// Status bar
	if m.loading {
		b.WriteString(sDim.Render("refreshing…"))
	} else {
		b.WriteString(sDim.Render(
			"last: " + m.lastFetch.Format("15:04:05") +
				"  •  r=refresh  q=quit",
		))
	}
	b.WriteByte('\n')

	return sBorder.Render(b.String())
}

// ─── Entry point ─────────────────────────────────────────────────────────────

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}
	creds, err := loadCredentials()
	if err != nil {
		fmt.Fprintln(os.Stderr, "credentials error:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(newModel(cfg, creds), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
