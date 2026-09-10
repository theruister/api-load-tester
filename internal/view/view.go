package view

import (
	"fmt"
	"load-tester/internal/stats"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/bubbles/progress"
)

type tickMsg time.Time

type model struct {
	recorder *stats.Recorder
	start    time.Time
	duration time.Duration
	stats    stats.Summary
	progress progress.Model
}

func InitialModel(r *stats.Recorder, t time.Time, d time.Duration) model {
	return model{recorder: r,
		start:    t,
		duration: d,
		progress: progress.New(progress.WithDefaultGradient()),
	}
}

func (m model) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		elapsed := time.Since(m.start)
		m.stats = m.recorder.Snapshot(elapsed, m.duration)
		return m, tickCmd()
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1).
			MarginBottom(1)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")). // gray
			Width(12)

	valueStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")) // aqua

	errorValueStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("203")) // red

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)
)

func (m model) View() tea.View {
	row := func(label string, value string, style lipgloss.Style) string {
		return lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render(label), style.Render(value))
	}

	body := strings.Join([]string{
		row("Total", fmt.Sprintf("%d", m.stats.Total), valueStyle),
		row("Successes", fmt.Sprintf("%d", m.stats.Successes), valueStyle),
		row("Errors", fmt.Sprintf("%d", m.stats.Errors), valueStyle),
		row("Duration", fmt.Sprintf("%s", m.stats.Duration), valueStyle),
		row("Throughput", fmt.Sprintf("%.1f", m.stats.Throughput), valueStyle),
		row("Min Latency", fmt.Sprintf("%s", m.stats.MinLatency), valueStyle),
		row("Avg Latency", fmt.Sprintf("%s", m.stats.AvgLatency), valueStyle),
		row("p50 Latency", fmt.Sprintf("%s", m.stats.P50Latency), valueStyle),
		row("p95 Latency", fmt.Sprintf("%s", m.stats.P95Latency), valueStyle),
		row("p99 Latency", fmt.Sprintf("%s", m.stats.P99Latency), valueStyle),
		row("Max Latency", fmt.Sprintf("%s", m.stats.MaxLatency), valueStyle),
	}, "\n")

	percent := float64(m.stats.Duration.Seconds() / m.duration.Seconds())

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Live Stats"),
		boxStyle.Render(body),
		m.progress.ViewAs(percent),
		helpStyle.Render("press q to quit"),
	)

	return tea.NewView(content)
}
