package dashboard

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/coetzeevs/cerebro/internal/metrics"
)

const (
	tabOverview = iota
	tabTurns
	tabDetail
	tabTools
	tabTrends
	tabCount // sentinel for modulo
)

var tabNames = []string{"Overview", "Turns", "Detail", "Tools", "Trends"}

// Config holds the dashboard configuration.
type Config struct {
	MetricsStore *metrics.MetricsStore
	BrainDB      *sql.DB // read-only handle to brain.sqlite
	SessionDir   string  // Claude Code JSONL directory for live refresh
	LastN        int     // number of recent turns to display
}

// Model is the top-level Bubble Tea model for the dashboard.
type Model struct {
	config    Config
	activeTab int
	width     int
	height    int

	// Data.
	turns    []metrics.TurnMetrics
	overview OverviewData
	table    table.Model

	// State.
	err        error
	lastIngest time.Time
}

// New creates a new dashboard model.
func New(cfg Config) Model {
	if cfg.LastN == 0 {
		cfg.LastN = 200
	}
	return Model{
		config: cfg,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { //nolint:gocritic // tea.Model requires value receiver
	return tea.Batch(
		m.loadData,
		tickCmd(),
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocritic // tea.Model requires value receiver
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table = BuildTurnsTable(m.turns, m.width, m.height-4)
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "1":
			m.activeTab = tabOverview
		case "2":
			m.activeTab = tabTurns
		case "3":
			m.activeTab = tabDetail
		case "4":
			m.activeTab = tabTools
		case "5":
			m.activeTab = tabTrends
		case "tab", "right":
			m.activeTab = (m.activeTab + 1) % tabCount
		case "shift+tab", "left":
			m.activeTab = (m.activeTab - 1 + tabCount) % tabCount
		case "r":
			return m, m.loadData
		default:
			// Forward to active panel.
			if m.activeTab == tabTurns {
				var cmd tea.Cmd
				m.table, cmd = m.table.Update(msg)
				return m, cmd
			}
		}
		return m, nil

	case dataLoadedMsg:
		m.turns = msg.turns
		m.overview = msg.overview
		m.lastIngest = time.Now()
		if m.width > 0 {
			m.table = BuildTurnsTable(m.turns, m.width, m.height-4)
		}
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case tickMsg:
		// Live refresh: re-ingest and reload data.
		return m, tea.Batch(m.liveIngest, tickCmd())

	default:
		// Forward to turns table for mouse/scroll events.
		if m.activeTab == tabTurns {
			var cmd tea.Cmd
			m.table, cmd = m.table.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// View implements tea.Model.
func (m Model) View() tea.View { //nolint:gocritic // tea.Model requires value receiver
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	var b strings.Builder

	// Tab bar.
	b.WriteString(m.renderTabBar())
	b.WriteString("\n")

	// Active panel content.
	contentHeight := m.height - 4 // tab bar + help line
	switch m.activeTab {
	case tabOverview:
		b.WriteString(RenderOverview(&m.overview, m.width, contentHeight))
	case tabTurns:
		b.WriteString(m.table.View())
	case tabDetail, tabTools, tabTrends:
		b.WriteString(contentStyle.Render(
			fmt.Sprintf("\n  Panel %d (%s) — coming in next release.\n\n  Use Overview (1) and Turns (2) panels.",
				m.activeTab+1, tabNames[m.activeTab])))
	}

	// Error display.
	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(warningStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
	}

	// Help line.
	b.WriteString("\n")
	lastRefresh := ""
	if !m.lastIngest.IsZero() {
		lastRefresh = fmt.Sprintf(" | refreshed %s", m.lastIngest.Format("15:04:05"))
	}
	b.WriteString(helpStyle.Render(fmt.Sprintf(
		"  1-5: panels  tab/arrows: navigate  r: refresh  q: quit%s", lastRefresh)))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m Model) renderTabBar() string { //nolint:gocritic // used by View()
	var tabs []string
	for i, name := range tabNames {
		label := fmt.Sprintf(" %d:%s ", i+1, name)
		if i == m.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}

	title := titleStyle.Render("Cerebro Dashboard")
	tabBar := strings.Join(tabs, "")

	// Right-align the tab bar.
	padding := m.width - len(title) - len(tabBar) - 4
	if padding < 1 {
		padding = 1
	}

	return title + strings.Repeat(" ", padding) + tabBar
}

// --- Messages ---

type dataLoadedMsg struct {
	turns    []metrics.TurnMetrics
	overview OverviewData
}

type errMsg struct {
	err error
}

type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

// --- Commands ---

func (m Model) loadData() tea.Msg { //nolint:gocritic // tea.Cmd function
	turns, err := m.config.MetricsStore.QueryTurns(metrics.TurnFilter{
		Limit:     m.config.LastN,
		OrderDesc: true,
	})
	if err != nil {
		return errMsg{err: err}
	}

	// Reverse for chronological order (oldest first).
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}

	overview := ComputeOverview(turns)

	// Brain health from brain DB.
	if m.config.BrainDB != nil {
		overview.BrainNodes = queryInt(m.config.BrainDB, "SELECT COUNT(*) FROM nodes WHERE status='active'")
		overview.BrainEdges = queryInt(m.config.BrainDB, "SELECT COUNT(*) FROM edges")
		overview.BrainColdNodes = queryInt(m.config.BrainDB,
			"SELECT COUNT(*) FROM nodes WHERE status='active' AND (last_surfaced IS NULL OR last_surfaced < datetime('now', '-7 days'))")

		rows, err := m.config.BrainDB.Query("SELECT type, COUNT(*) FROM nodes WHERE status='active' GROUP BY type")
		if err == nil {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var ntype string
				var count int
				if rows.Scan(&ntype, &count) == nil {
					overview.BrainByType[ntype] = count
				}
			}
		}
	}

	return dataLoadedMsg{
		turns:    turns,
		overview: overview,
	}
}

func (m Model) liveIngest() tea.Msg { //nolint:gocritic // tea.Cmd function
	// Incremental ingest: parse any new JSONL data.
	if m.config.SessionDir != "" {
		ingestFromDir(m.config.MetricsStore, m.config.SessionDir)
	}
	return m.loadData()
}

// queryInt runs a single-row integer query.
func queryInt(db *sql.DB, query string) int {
	var n int
	_ = db.QueryRow(query).Scan(&n)
	return n
}
