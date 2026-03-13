package ui

import "charm.land/lipgloss/v2"

type Styles struct {
	App        lipgloss.Style
	Panel      lipgloss.Style
	Focused    lipgloss.Style
	Title      lipgloss.Style
	Dim        lipgloss.Style
	Warn       lipgloss.Style
	Error      lipgloss.Style
	BarFull    lipgloss.Style
	BarEmpty   lipgloss.Style
	Selected   lipgloss.Style
	StatusLine lipgloss.Style
}

func NewStyles() Styles {
	return Styles{
		App: lipgloss.NewStyle().
			Padding(2, 3).
			Foreground(lipgloss.Color("#EDE6D6")).
			Background(lipgloss.Color("#11161B")),
		Panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4F6D7A")).
			Padding(1, 2),
		Focused: lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("#D17B0F")).
			Padding(1, 2),
		Title: lipgloss.NewStyle().
			Bold(true).
			PaddingBottom(1).
			Foreground(lipgloss.Color("#F4B860")),
		Dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#91A3B0")),
		Warn: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F4B860")),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F87060")),
		BarFull: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7BD389")),
		BarEmpty: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#2D3A45")),
		Selected: lipgloss.NewStyle().
			Background(lipgloss.Color("#22303A")).
			Padding(0, 1).
			Foreground(lipgloss.Color("#F8F4EC")),
		StatusLine: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C9D6DF")),
	}
}
