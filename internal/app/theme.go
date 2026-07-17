package app

import "github.com/charmbracelet/lipgloss"

// Theme holds the color palette for one visual theme. Hex values are
// pre-converted from the prototype's oklch() colors to sRGB.
type Theme struct {
	Name   string
	FG     lipgloss.Color
	Accent lipgloss.Color
	Danger lipgloss.Color
	Dim    lipgloss.Color
	Dim2   lipgloss.Color
}

var themes = []Theme{
	{
		Name:   "ocean",
		FG:     lipgloss.Color("#6CBAFF"),
		Accent: lipgloss.Color("#FF9B50"), Danger: lipgloss.Color("#FA6863"),
		Dim: lipgloss.Color("#55749A"), Dim2: lipgloss.Color("#7DA1D0"),
	},
	{
		Name:   "sunset",
		FG:     lipgloss.Color("#FF8940"),
		Accent: lipgloss.Color("#FF78BE"), Danger: lipgloss.Color("#FF5F5B"),
		Dim: lipgloss.Color("#956452"), Dim2: lipgloss.Color("#CF8B74"),
	},
	{
		Name:   "matrix",
		FG:     lipgloss.Color("#75D78D"),
		Accent: lipgloss.Color("#ECA851"), Danger: lipgloss.Color("#FA6863"),
		Dim: lipgloss.Color("#5D7A62"), Dim2: lipgloss.Color("#89A88E"),
	},
	{
		Name:   "banana",
		FG:     lipgloss.Color("#F4D916"),
		Accent: lipgloss.Color("#FF8600"), Danger: lipgloss.Color("#FF5F5B"),
		Dim: lipgloss.Color("#7C7148"), Dim2: lipgloss.Color("#AE9E64"),
	},
	{
		Name:   "bubblegum",
		FG:     lipgloss.Color("#FF7DDF"),
		Accent: lipgloss.Color("#00DBD3"), Danger: lipgloss.Color("#FF5F5B"),
		Dim: lipgloss.Color("#8C6088"), Dim2: lipgloss.Color("#C287BC"),
	},
	{
		Name:   "vaporwave",
		FG:     lipgloss.Color("#00DFE8"),
		Accent: lipgloss.Color("#FF7FE9"), Danger: lipgloss.Color("#FF6661"),
		Dim: lipgloss.Color("#6A6E94"), Dim2: lipgloss.Color("#9499D0"),
	},
	{
		Name:   "toxic",
		FG:     lipgloss.Color("#A1F400"),
		Accent: lipgloss.Color("#C38BFF"), Danger: lipgloss.Color("#FF5F5B"),
		Dim: lipgloss.Color("#647A4E"), Dim2: lipgloss.Color("#8CAA6E"),
	},
}

// IsValidTheme reports whether name matches one of the built-in themes.
func IsValidTheme(name string) bool {
	for _, t := range themes {
		if t.Name == name {
			return true
		}
	}
	return false
}

func themeByName(name string) Theme {
	for _, t := range themes {
		if t.Name == name {
			return t
		}
	}
	return themes[0]
}

func nextTheme(name string) Theme {
	for i, t := range themes {
		if t.Name == name {
			return themes[(i+1)%len(themes)]
		}
	}
	return themes[0]
}

func prevTheme(name string) Theme {
	for i, t := range themes {
		if t.Name == name {
			return themes[(i-1+len(themes))%len(themes)]
		}
	}
	return themes[0]
}
