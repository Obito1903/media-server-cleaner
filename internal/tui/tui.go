package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/obito1903/media-server-cleaner/internal/media"
)

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	tabStyle       = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("245"))
	activeTabStyle = tabStyle.Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(true)
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	warnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	protStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	headStyle      = lipgloss.NewStyle().Bold(true).Underline(true)
	sepStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

type sortMode int

const (
	sortScore sortMode = iota
	sortSize
	sortLastPlayed
	sortTitle
	sortCount
)

var sortNames = [...]string{"score", "size", "last played", "title"}

type model struct {
	all, view     []*media.Item
	libs          []string
	lib           int
	sort          sortMode
	hideProtected bool
	tvSeasons     bool // score TV per season instead of per show
	fullDetail    bool
	warnings      []string
	now           time.Time

	table     table.Model
	detail    viewport.Model
	detailKey string
	width     int
	height    int
}

func Run(items []*media.Item, warnings []string) error {
	_, err := tea.NewProgram(newModel(items, warnings), tea.WithAltScreen()).Run()
	return err
}

func newModel(items []*media.Item, warnings []string) *model {
	libSet := map[string]bool{}
	for _, it := range items {
		libSet[it.Library] = true
	}
	libs := []string{"All"}
	for l := range libSet {
		libs = append(libs, l)
	}
	sort.Strings(libs[1:])

	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).BorderStyle(lipgloss.NormalBorder()).BorderBottom(true)
	st.Selected = st.Selected.Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))

	m := &model{
		all:      items,
		libs:     libs,
		warnings: warnings,
		now:      time.Now(),
		table:    table.New(table.WithColumns(columns(80)), table.WithFocused(true), table.WithStyles(st)),
		detail:   viewport.New(0, 0),
	}
	m.refresh()
	return m
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.lib = (m.lib + 1) % len(m.libs)
			m.refresh()
			return m, nil
		case "shift+tab":
			m.lib = (m.lib - 1 + len(m.libs)) % len(m.libs)
			m.refresh()
			return m, nil
		case "s":
			m.sort = (m.sort + 1) % sortCount
			m.refresh()
			return m, nil
		case "p":
			m.hideProtected = !m.hideProtected
			m.refresh()
			return m, nil
		case "t":
			m.tvSeasons = !m.tvSeasons
			m.refresh()
			return m, nil
		case "enter":
			m.fullDetail = !m.fullDetail
			m.layout()
			return m, nil
		case "esc":
			if m.fullDetail {
				m.fullDetail = false
				m.layout()
			}
			return m, nil
		}
		if m.fullDetail {
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(msg)
			return m, cmd
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	m.updateDetail()
	return m, cmd
}

func (m *model) layout() {
	avail := max(m.height-3, 4) // header, status, help
	m.detail.Width = m.width
	if m.fullDetail {
		m.detail.Height = avail
	} else {
		th := avail * 55 / 100
		m.table.SetHeight(th)
		m.detail.Height = avail - th - 1
	}
	m.table.SetWidth(m.width)
	m.table.SetColumns(columns(m.width))
	m.detailKey = ""
	m.updateDetail()
}

func columns(width int) []table.Column {
	cols := []table.Column{
		{Title: "Score", Width: 6},
		{Title: "Title", Width: 0},
		{Title: "Year", Width: 4},
		{Title: "Library", Width: 14},
		{Title: "Size", Width: 9},
		{Title: "Plays", Width: 5},
		{Title: "Viewers", Width: 7},
		{Title: "Last played", Width: 11},
		{Title: "Req", Width: 3},
	}
	used := 0
	for _, c := range cols {
		used += c.Width + 2 // cell padding
	}
	cols[1].Width = max(width-used-1, 20)
	return cols
}

func (m *model) refresh() {
	m.view = m.view[:0]
	for _, it := range m.all {
		if m.lib > 0 && it.Library != m.libs[m.lib] {
			continue
		}
		if m.hideProtected && it.Protected != "" {
			continue
		}
		if (m.tvSeasons && it.Type == "show") || (!m.tvSeasons && it.Type == "season") {
			continue
		}
		m.view = append(m.view, it)
	}
	sort.SliceStable(m.view, func(i, j int) bool {
		a, b := m.view[i], m.view[j]
		switch m.sort {
		case sortSize:
			return a.SizeBytes > b.SizeBytes
		case sortLastPlayed:
			return a.LastPlayed.Before(b.LastPlayed)
		case sortTitle:
			return strings.ToLower(a.DisplayTitle()) < strings.ToLower(b.DisplayTitle())
		}
		return a.Score > b.Score
	})

	rows := make([]table.Row, len(m.view))
	for i, it := range m.view {
		score := fmt.Sprintf("%5.1f", it.Score)
		if it.Protected != "" {
			score = "  0 P"
		}
		last := "never"
		if !it.LastPlayed.IsZero() {
			last = it.LastPlayed.Format("2006-01-02")
		}
		req := ""
		if len(it.Requests) > 0 {
			req = "yes"
		}
		rows[i] = table.Row{
			score, it.DisplayTitle(), yearStr(it.Year), it.Library, media.HumanBytes(it.SizeBytes),
			fmt.Sprint(it.Plays), fmt.Sprint(len(it.Users)), last, req,
		}
	}
	m.table.SetRows(rows)
	if m.table.Cursor() >= len(rows) || m.table.Cursor() < 0 {
		m.table.SetCursor(0)
	}
	m.detailKey = ""
	m.updateDetail()
}

func (m *model) updateDetail() {
	if len(m.view) == 0 {
		m.detail.SetContent(dimStyle.Render("No items."))
		return
	}
	it := m.view[m.table.Cursor()]
	if it.RatingKey == m.detailKey {
		return
	}
	m.detailKey = it.RatingKey
	m.detail.SetContent(lipgloss.NewStyle().Width(max(m.width, 20)).Render(renderDetail(it, m.now)))
	m.detail.GotoTop()
}

func (m *model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	var tabs []string
	for i, l := range m.libs {
		if i == m.lib {
			tabs = append(tabs, activeTabStyle.Render(l))
		} else {
			tabs = append(tabs, tabStyle.Render(l))
		}
	}
	header := titleStyle.Render("Media Cleaner ") + strings.Join(tabs, "")

	var total int64
	for _, it := range m.view {
		total += it.SizeBytes
	}
	tv := "whole shows"
	if m.tvSeasons {
		tv = "seasons"
	}
	status := dimStyle.Render(fmt.Sprintf("%d items · %s · TV: %s · sort: %s · hide protected: %v",
		len(m.view), media.HumanBytes(total), tv, sortNames[m.sort], m.hideProtected))
	if len(m.warnings) > 0 {
		status += "  " + warnStyle.Render("⚠ "+strings.Join(m.warnings, " | "))
	}

	help := dimStyle.Render("↑/↓ move · tab/shift+tab library · t shows/seasons · s sort · p hide protected · enter details · q quit")
	if m.fullDetail {
		help = dimStyle.Render("↑/↓/pgup/pgdn scroll · esc/enter back · q quit")
		return lipgloss.JoinVertical(lipgloss.Left, header, status, m.detail.View(), help)
	}
	sep := sepStyle.Render(strings.Repeat("─", m.width))
	return lipgloss.JoinVertical(lipgloss.Left, header, status, m.table.View(), sep, m.detail.View(), help)
}

func yearStr(y int) string {
	if y == 0 {
		return ""
	}
	return fmt.Sprint(y)
}

func renderDetail(it *media.Item, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s", titleStyle.Render(it.DisplayTitle()))
	if it.Year > 0 {
		fmt.Fprintf(&b, " (%d)", it.Year)
	}
	fmt.Fprintf(&b, "  %s\n", dimStyle.Render(it.Library+" · "+it.Type+" · ratingKey "+it.RatingKey))

	fmt.Fprintf(&b, "Added %s (%.0fd ago) · Size %s", it.AddedAt.Format("2006-01-02"), media.DaysSince(it.AddedAt, now), media.HumanBytes(it.SizeBytes))
	if it.Type != "movie" {
		fmt.Fprintf(&b, " · %d episodes", it.LeafCount)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "Score %.1f / 100", it.Score)
	if it.Protected != "" {
		fmt.Fprintf(&b, "  %s", protStyle.Render(fmt.Sprintf("PROTECTED: %s — raw score %.1f", it.Protected, it.RawScore)))
	}
	b.WriteString("\n\n")

	b.WriteString(headStyle.Render("Score breakdown") + dimStyle.Render("  (higher = better deletion candidate)") + "\n")
	var sum, maxSum float64
	for _, f := range it.Factors {
		fmt.Fprintf(&b, "  %-21s %5.1f/%-4.0f  %s\n", f.Name, f.Points, f.Max, dimStyle.Render(f.Calc))
		sum += f.Points
		maxSum += f.Max
	}
	fmt.Fprintf(&b, "  %-21s %5.1f/%-4.0f  %s\n\n", "Total", sum, maxSum,
		dimStyle.Render(fmt.Sprintf("normalized: %.1f / %.0f × 100 = %.1f", sum, maxSum, it.RawScore)))

	b.WriteString(headStyle.Render("Viewers") + "\n")
	if len(it.Users) == 0 {
		b.WriteString(dimStyle.Render("  none") + "\n")
	} else {
		users := make([]string, 0, len(it.Users))
		for u := range it.Users {
			users = append(users, u)
		}
		sort.Slice(users, func(i, j int) bool { return it.Users[users[i]] > it.Users[users[j]] })
		for _, u := range users {
			fmt.Fprintf(&b, "  %s (%d plays)\n", u, it.Users[u])
		}
		fmt.Fprintf(&b, "  last played %s\n", it.LastPlayed.Format("2006-01-02 15:04"))
		fmt.Fprintf(&b, "  %s\n", dimStyle.Render(fmt.Sprintf("%d plays from Tautulli, %d from Plex history (before Tautulli)", it.Plays-it.PlexPlays, it.PlexPlays)))
	}

	b.WriteString("\n" + headStyle.Render("Seerr requests") + "\n")
	if len(it.Requests) == 0 {
		b.WriteString(dimStyle.Render("  none") + "\n")
	}
	for _, r := range it.Requests {
		fmt.Fprintf(&b, "  %s on %s (%s)\n", r.User, r.CreatedAt.Format("2006-01-02"), r.Status)
	}
	return b.String()
}
