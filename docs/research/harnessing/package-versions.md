# TUI Package Research: Versions and API Patterns

> Researched 2026-04-23 via Context7 + Go module proxy

---

## 1. charmbracelet/bubbletea

| Field | Value |
|-------|-------|
| Latest version | **v2.0.6** (2026-04-16) |
| Go module path | `github.com/charmbracelet/bubbletea/v2` |
| Import path | `tea "charm.land/bubbletea/v2"` |
| Install | `go get charm.land/bubbletea/v2@latest` |

### v2 Migration Notes

Bubbletea v2 is stable (past beta). Key breaking changes from v1:

- **Import path changed**: `github.com/charmbracelet/bubbletea` -> `charm.land/bubbletea/v2`
- **`View()` returns `tea.View`** instead of `string`:
  ```go
  // v1
  func (m model) View() string { return "Hello" }
  // v2
  func (m model) View() tea.View { return tea.NewView("Hello") }
  ```
- **`tea.KeyMsg` -> `tea.KeyPressMsg`**: `tea.KeyMsg` is now an interface; use `tea.KeyPressMsg` for key press events.
- **Program options moved to View**: Alt screen, mouse mode etc. are now set on the View struct rather than `tea.NewProgram()` options.
  ```go
  // v1: tea.NewProgram(model{}, tea.WithAltScreen(), tea.WithMouseCellMotion())
  // v2: set in View()
  v := tea.NewView(content)
  v.AltScreen = true
  v.MouseMode = tea.MouseModeCellMotion
  ```
- **Mouse events**: `tea.MouseMsg` -> `tea.MouseClickMsg`, `msg.Button == tea.MouseLeft`
- **Key string change**: `" "` (space) -> `"space"`

### Minimal v2 Example

```go
package main

import (
    "fmt"
    "os"

    tea "charm.land/bubbletea/v2"
)

type model struct {
    cursor  int
    choices []string
    selected map[int]struct{}
}

func initialModel() model {
    return model{
        choices:  []string{"Buy carrots", "Buy celery", "Buy kohlrabi"},
        selected: make(map[int]struct{}),
    }
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyPressMsg:
        switch msg.String() {
        case "ctrl+c", "q":
            return m, tea.Quit
        case "up", "k":
            if m.cursor > 0 { m.cursor-- }
        case "down", "j":
            if m.cursor < len(m.choices)-1 { m.cursor++ }
        case "enter", "space":
            if _, ok := m.selected[m.cursor]; ok {
                delete(m.selected, m.cursor)
            } else {
                m.selected[m.cursor] = struct{}{}
            }
        }
    }
    return m, nil
}

func (m model) View() tea.View {
    s := "What should we buy?\n\n"
    for i, choice := range m.choices {
        cursor := " "
        if m.cursor == i { cursor = ">" }
        checked := " "
        if _, ok := m.selected[i]; ok { checked = "x" }
        s += fmt.Sprintf("%s [%s] %s\n", cursor, checked, choice)
    }
    s += "\nPress q to quit.\n"
    return tea.NewView(s)
}

func main() {
    p := tea.NewProgram(initialModel())
    if _, err := p.Run(); err != nil {
        fmt.Printf("Error: %v\n", err)
        os.Exit(1)
    }
}
```

---

## 2. charmbracelet/lipgloss

| Field | Value |
|-------|-------|
| Latest version | **v2.0.3** (2026-04-13) |
| Go module path | `github.com/charmbracelet/lipgloss/v2` |
| Import path | `"charm.land/lipgloss/v2"` |
| Table sub-package | `"charm.land/lipgloss/v2/table"` |
| Install | `go get charm.land/lipgloss/v2@latest` |

### v2 Migration Notes

- Import path changed from `github.com/charmbracelet/lipgloss` to `charm.land/lipgloss/v2`
- Style is a value type (copy by assignment)
- `lipgloss.Println()` auto-downgrades colors for the terminal

### Styling Basics

```go
style := lipgloss.NewStyle().
    Bold(true).
    Foreground(lipgloss.Color("#FAFAFA")).
    Background(lipgloss.Color("#7D56F4")).
    Padding(1, 2).
    Width(40).
    Align(lipgloss.Center).
    Border(lipgloss.RoundedBorder()).
    BorderForeground(lipgloss.Color("#874BFD"))

lipgloss.Println(style.Render("Hello, World!"))
```

### Table Rendering

```go
package main

import (
    "charm.land/lipgloss/v2"
    "charm.land/lipgloss/v2/table"
)

func main() {
    purple := lipgloss.Color("99")
    gray := lipgloss.Color("245")

    headerStyle := lipgloss.NewStyle().
        Foreground(purple).
        Bold(true).
        Align(lipgloss.Center)

    cellStyle := lipgloss.NewStyle().Padding(0, 1)
    evenRow := cellStyle.Foreground(lipgloss.Color("250"))
    oddRow := cellStyle.Foreground(gray)

    t := table.New().
        Border(lipgloss.RoundedBorder()).
        BorderStyle(lipgloss.NewStyle().Foreground(purple)).
        Headers("NAME", "AGE", "CITY").
        Row("Alice", "30", "New York").
        Row("Bob", "25", "San Francisco").
        StyleFunc(func(row, col int) lipgloss.Style {
            switch {
            case row == table.HeaderRow:
                return headerStyle
            case row%2 == 0:
                return evenRow
            default:
                return oddRow
            }
        }).
        Width(50)

    lipgloss.Println(t)
}
```

Available border types: `NormalBorder()`, `RoundedBorder()`, `ThickBorder()`, `MarkdownBorder()`

---

## 3. charmbracelet/bubbles

| Field | Value |
|-------|-------|
| Latest version | **v2.1.0** (2026-03-25) |
| Go module path | `github.com/charmbracelet/bubbles/v2` |
| Import path | `"charm.land/bubbles/v2/<component>"` |
| Install | `go get charm.land/bubbles/v2@latest` |

### Available Components

| Component | Import | Description |
|-----------|--------|-------------|
| cursor | `charm.land/bubbles/v2/cursor` | Text cursor |
| filepicker | `charm.land/bubbles/v2/filepicker` | File picker |
| help | `charm.land/bubbles/v2/help` | Help view with keybindings |
| key | `charm.land/bubbles/v2/key` | Key binding definitions |
| list | `charm.land/bubbles/v2/list` | Interactive list with filtering |
| paginator | `charm.land/bubbles/v2/paginator` | Page navigation |
| progress | `charm.land/bubbles/v2/progress` | Progress bar |
| spinner | `charm.land/bubbles/v2/spinner` | Loading spinner |
| stopwatch | `charm.land/bubbles/v2/stopwatch` | Stopwatch timer |
| table | `charm.land/bubbles/v2/table` | Tabular data with scrolling |
| textarea | `charm.land/bubbles/v2/textarea` | Multi-line text input |
| textinput | `charm.land/bubbles/v2/textinput` | Single-line text input |
| timer | `charm.land/bubbles/v2/timer` | Countdown timer |
| viewport | `charm.land/bubbles/v2/viewport` | Scrollable content viewport |

### v2 Migration Notes

- **Import paths**: `github.com/charmbracelet/bubbles/<comp>` -> `charm.land/bubbles/v2/<comp>`
- **Width/Height fields -> getter/setter methods** (applies to filepicker, help, progress, table, textinput, viewport):
  ```go
  // v1: vp.Width = 80; vp.Height = 24
  // v2:
  vp.SetWidth(80)
  vp.SetHeight(24)
  fmt.Println(vp.Width(), vp.Height())
  ```
- **Viewport constructor uses functional options**:
  ```go
  // v1: vp := viewport.New(80, 24)
  // v2:
  vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(24))
  ```
- **DefaultKeyMap is now a function**: `textinput.DefaultKeyMap` -> `textinput.DefaultKeyMap()`
- **Progress gradient API changed**:
  ```go
  // v1: progress.New(progress.WithGradient("#5A56E0", "#EE6FF8"))
  // v2: progress.New(progress.WithColors(lipgloss.Color("#5A56E0"), lipgloss.Color("#EE6FF8")))
  ```

---

## 4. guptarohit/asciigraph

| Field | Value |
|-------|-------|
| Latest version | **v0.9.0** (2026-03-28) |
| Go module path | `github.com/guptarohit/asciigraph` |
| Import path | `"github.com/guptarohit/asciigraph"` |
| Install | `go get github.com/guptarohit/asciigraph@latest` |

No v2 -- still on v0.x. No vanity import path.

### API: Plot (single series)

```go
func Plot(data []float64, options ...Option) string
```

```go
graph := asciigraph.Plot(data,
    asciigraph.Height(8),
    asciigraph.Width(40),
    asciigraph.Caption("Sample Data"),
    asciigraph.Precision(2),
)
fmt.Println(graph)
```

### API: PlotMany (multiple series)

```go
func PlotMany(series [][]float64, options ...Option) string
```

```go
graph := asciigraph.PlotMany(
    [][]float64{series1, series2},
    asciigraph.Height(6),
    asciigraph.SeriesColors(asciigraph.Blue, asciigraph.Red),
    asciigraph.SeriesLegends("Series A", "Series B"),
    asciigraph.Caption("Multi-series comparison"),
)
fmt.Println(graph)
```

### Available Options

| Option | Description |
|--------|-------------|
| `Height(int)` | Graph height in rows |
| `Width(int)` | Graph width in columns |
| `Caption(string)` | Caption below the graph |
| `Precision(uint)` | Decimal places for Y-axis labels |
| `Offset(int)` | Left margin width |
| `SeriesColors(...Color)` | ANSI colors per series |
| `SeriesLegends(...string)` | Legend labels per series |
| `CaptionColor(Color)` | Color of the caption |
| `AxisColor(Color)` | Color of the axes |
| `LabelColor(Color)` | Color of Y-axis labels |

Available colors: `Red`, `Blue`, `Green`, `Yellow`, `Cyan`, `DarkGray`, and 140+ named ANSI colors.

---

## Version Summary

| Package | Version | Module Path | Vanity Import |
|---------|---------|-------------|---------------|
| bubbletea | v2.0.6 | `github.com/charmbracelet/bubbletea/v2` | `charm.land/bubbletea/v2` |
| lipgloss | v2.0.3 | `github.com/charmbracelet/lipgloss/v2` | `charm.land/lipgloss/v2` |
| bubbles | v2.1.0 | `github.com/charmbracelet/bubbles/v2` | `charm.land/bubbles/v2` |
| asciigraph | v0.9.0 | `github.com/guptarohit/asciigraph` | N/A |

### go get commands

```bash
go get charm.land/bubbletea/v2@v2.0.6
go get charm.land/lipgloss/v2@v2.0.3
go get charm.land/bubbles/v2@v2.1.0
go get github.com/guptarohit/asciigraph@v0.9.0
```

### Key cross-cutting note

All three Charm packages moved to `charm.land` vanity imports in v2. The old `github.com/charmbracelet/*` paths still work as Go module paths for `go get`, but source code imports should use `charm.land/*` for v2.
