package app

import (
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
)

type appTheme struct {
	Name          string
	Background    string
	Panel         string
	Border        string
	Title         string
	Primary       string
	Secondary     string
	Dim           string
	ActiveTabFG   string
	ActiveTabBG   string
	SelectionFG   string
	SelectionBG   string
	StatusAccent  string
	CommandAccent string
	Syntax        map[string]string
}

func currentTheme() appTheme {
	theme, ok := themesByName[settings.CurrentTheme()]
	if !ok {
		return themesByName[settings.DefaultTheme]
	}
	return theme
}

func themeColor(hex string) tcell.Color {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return tcell.ColorDefault
	}
	value, err := strconv.ParseInt(hex, 16, 32)
	if err != nil {
		return tcell.ColorDefault
	}
	return tcell.NewHexColor(int32(value))
}

func themeMarkupFG(hex string) string {
	return "[" + hex + "]"
}

func themeMarkupFGStyle(hex string, style string) string {
	return "[" + hex + "::" + style + "]"
}

func themeMarkupFGStyleBG(fg string, bg string, style string) string {
	return "[" + fg + ":" + bg + ":" + style + "]"
}

func themeMarkupPair(fg string, bg string) string {
	return "[" + fg + ":" + bg + "]"
}

var themesByName = map[string]appTheme{
	"tokyo-night": {
		Name:          "tokyo-night",
		Background:    "#1a1b26",
		Panel:         "#16161e",
		Border:        "#565f89",
		Title:         "#7dcfff",
		Primary:       "#c0caf5",
		Secondary:     "#a9b1d6",
		Dim:           "#737aa2",
		ActiveTabFG:   "#1a1b26",
		ActiveTabBG:   "#c0caf5",
		SelectionFG:   "#c0caf5",
		SelectionBG:   "#364a82",
		StatusAccent:  "#9ece6a",
		CommandAccent: "#7dcfff",
		Syntax: map[string]string{
			helpers.ANSIRoleHeading1: "#7dcfff", helpers.ANSIRoleHeading2: "#bb9af7", helpers.ANSIRoleHeading3: "#e0af68",
			helpers.ANSIRoleHeading4: "#7aa2f7", helpers.ANSIRoleHeading5: "#ff9e64", helpers.ANSIRoleHeading6: "#9ece6a",
			helpers.ANSIRoleListMarker: "#737aa2", helpers.ANSIRoleLink: "#e0af68", helpers.ANSIRoleCode: "#9ece6a",
			helpers.ANSIRoleString: "#9ece6a", helpers.ANSIRoleKeyword: "#bb9af7", helpers.ANSIRoleNumber: "#ff9e64",
			helpers.ANSIRoleComment: "#565f89", helpers.ANSIRoleType: "#7dcfff", helpers.ANSIRoleFunction: "#7aa2f7",
			helpers.ANSIRoleProperty: "#e0af68", helpers.ANSIRoleConstant: "#e0af68", helpers.ANSIRoleSearch: "#1a1b26",
			helpers.ANSIRoleVisualSelection: "#c0caf5", helpers.ANSIRoleActiveTab: "#1a1b26", helpers.ANSIRoleSelection: "#c0caf5",
		},
	},
	"catppuccin": {
		Name:          "catppuccin",
		Background:    "#1e1e2e",
		Panel:         "#181825",
		Border:        "#585b70",
		Title:         "#89b4fa",
		Primary:       "#cdd6f4",
		Secondary:     "#bac2de",
		Dim:           "#6c7086",
		ActiveTabFG:   "#1e1e2e",
		ActiveTabBG:   "#cdd6f4",
		SelectionFG:   "#cdd6f4",
		SelectionBG:   "#45475a",
		StatusAccent:  "#a6e3a1",
		CommandAccent: "#89dceb",
		Syntax: map[string]string{
			helpers.ANSIRoleHeading1: "#89b4fa", helpers.ANSIRoleHeading2: "#cba6f7", helpers.ANSIRoleHeading3: "#f9e2af",
			helpers.ANSIRoleHeading4: "#89dceb", helpers.ANSIRoleHeading5: "#fab387", helpers.ANSIRoleHeading6: "#a6e3a1",
			helpers.ANSIRoleListMarker: "#6c7086", helpers.ANSIRoleLink: "#f9e2af", helpers.ANSIRoleCode: "#a6e3a1",
			helpers.ANSIRoleString: "#a6e3a1", helpers.ANSIRoleKeyword: "#cba6f7", helpers.ANSIRoleNumber: "#fab387",
			helpers.ANSIRoleComment: "#7f849c", helpers.ANSIRoleType: "#89dceb", helpers.ANSIRoleFunction: "#89b4fa",
			helpers.ANSIRoleProperty: "#f9e2af", helpers.ANSIRoleConstant: "#f9e2af", helpers.ANSIRoleSearch: "#1e1e2e",
			helpers.ANSIRoleVisualSelection: "#cdd6f4", helpers.ANSIRoleActiveTab: "#1e1e2e", helpers.ANSIRoleSelection: "#cdd6f4",
		},
	},
	"kanagawa": {
		Name:          "kanagawa",
		Background:    "#1f1f28",
		Panel:         "#16161d",
		Border:        "#54546d",
		Title:         "#7e9cd8",
		Primary:       "#dcd7ba",
		Secondary:     "#c8c093",
		Dim:           "#727169",
		ActiveTabFG:   "#1f1f28",
		ActiveTabBG:   "#dcd7ba",
		SelectionFG:   "#dcd7ba",
		SelectionBG:   "#2d4f67",
		StatusAccent:  "#98bb6c",
		CommandAccent: "#7fb4ca",
		Syntax: map[string]string{
			helpers.ANSIRoleHeading1: "#7e9cd8", helpers.ANSIRoleHeading2: "#957fb8", helpers.ANSIRoleHeading3: "#e6c384",
			helpers.ANSIRoleHeading4: "#7fb4ca", helpers.ANSIRoleHeading5: "#ffa066", helpers.ANSIRoleHeading6: "#98bb6c",
			helpers.ANSIRoleListMarker: "#727169", helpers.ANSIRoleLink: "#e6c384", helpers.ANSIRoleCode: "#98bb6c",
			helpers.ANSIRoleString: "#98bb6c", helpers.ANSIRoleKeyword: "#957fb8", helpers.ANSIRoleNumber: "#ffa066",
			helpers.ANSIRoleComment: "#727169", helpers.ANSIRoleType: "#7fb4ca", helpers.ANSIRoleFunction: "#7e9cd8",
			helpers.ANSIRoleProperty: "#e6c384", helpers.ANSIRoleConstant: "#e6c384", helpers.ANSIRoleSearch: "#1f1f28",
			helpers.ANSIRoleVisualSelection: "#dcd7ba", helpers.ANSIRoleActiveTab: "#1f1f28", helpers.ANSIRoleSelection: "#dcd7ba",
		},
	},
	"gruvbox": {
		Name:          "gruvbox",
		Background:    "#282828",
		Panel:         "#1d2021",
		Border:        "#665c54",
		Title:         "#83a598",
		Primary:       "#ebdbb2",
		Secondary:     "#d5c4a1",
		Dim:           "#928374",
		ActiveTabFG:   "#282828",
		ActiveTabBG:   "#ebdbb2",
		SelectionFG:   "#ebdbb2",
		SelectionBG:   "#504945",
		StatusAccent:  "#b8bb26",
		CommandAccent: "#8ec07c",
		Syntax: map[string]string{
			helpers.ANSIRoleHeading1: "#83a598", helpers.ANSIRoleHeading2: "#d3869b", helpers.ANSIRoleHeading3: "#fabd2f",
			helpers.ANSIRoleHeading4: "#8ec07c", helpers.ANSIRoleHeading5: "#fe8019", helpers.ANSIRoleHeading6: "#b8bb26",
			helpers.ANSIRoleListMarker: "#928374", helpers.ANSIRoleLink: "#fabd2f", helpers.ANSIRoleCode: "#b8bb26",
			helpers.ANSIRoleString: "#b8bb26", helpers.ANSIRoleKeyword: "#d3869b", helpers.ANSIRoleNumber: "#fe8019",
			helpers.ANSIRoleComment: "#928374", helpers.ANSIRoleType: "#8ec07c", helpers.ANSIRoleFunction: "#83a598",
			helpers.ANSIRoleProperty: "#fabd2f", helpers.ANSIRoleConstant: "#fabd2f", helpers.ANSIRoleSearch: "#282828",
			helpers.ANSIRoleVisualSelection: "#ebdbb2", helpers.ANSIRoleActiveTab: "#282828", helpers.ANSIRoleSelection: "#ebdbb2",
		},
	},
	"rose-pine": {
		Name:          "rose-pine",
		Background:    "#191724",
		Panel:         "#1f1d2e",
		Border:        "#6e6a86",
		Title:         "#9ccfd8",
		Primary:       "#e0def4",
		Secondary:     "#908caa",
		Dim:           "#6e6a86",
		ActiveTabFG:   "#191724",
		ActiveTabBG:   "#e0def4",
		SelectionFG:   "#e0def4",
		SelectionBG:   "#403d52",
		StatusAccent:  "#31748f",
		CommandAccent: "#c4a7e7",
		Syntax: map[string]string{
			helpers.ANSIRoleHeading1: "#9ccfd8", helpers.ANSIRoleHeading2: "#c4a7e7", helpers.ANSIRoleHeading3: "#f6c177",
			helpers.ANSIRoleHeading4: "#ebbcba", helpers.ANSIRoleHeading5: "#eb6f92", helpers.ANSIRoleHeading6: "#31748f",
			helpers.ANSIRoleListMarker: "#6e6a86", helpers.ANSIRoleLink: "#f6c177", helpers.ANSIRoleCode: "#31748f",
			helpers.ANSIRoleString: "#31748f", helpers.ANSIRoleKeyword: "#c4a7e7", helpers.ANSIRoleNumber: "#eb6f92",
			helpers.ANSIRoleComment: "#6e6a86", helpers.ANSIRoleType: "#9ccfd8", helpers.ANSIRoleFunction: "#ebbcba",
			helpers.ANSIRoleProperty: "#f6c177", helpers.ANSIRoleConstant: "#f6c177", helpers.ANSIRoleSearch: "#191724",
			helpers.ANSIRoleVisualSelection: "#e0def4", helpers.ANSIRoleActiveTab: "#191724", helpers.ANSIRoleSelection: "#e0def4",
		},
	},
	"flexoki": {
		Name:          "flexoki",
		Background:    "#100f0f",
		Panel:         "#1c1b1a",
		Border:        "#575653",
		Title:         "#4385be",
		Primary:       "#cecdc3",
		Secondary:     "#b7b5ac",
		Dim:           "#878580",
		ActiveTabFG:   "#100f0f",
		ActiveTabBG:   "#cecdc3",
		SelectionFG:   "#cecdc3",
		SelectionBG:   "#343331",
		StatusAccent:  "#66800b",
		CommandAccent: "#24837b",
		Syntax: map[string]string{
			helpers.ANSIRoleHeading1: "#4385be", helpers.ANSIRoleHeading2: "#8b7ec8", helpers.ANSIRoleHeading3: "#ad8301",
			helpers.ANSIRoleHeading4: "#24837b", helpers.ANSIRoleHeading5: "#bc5215", helpers.ANSIRoleHeading6: "#66800b",
			helpers.ANSIRoleListMarker: "#878580", helpers.ANSIRoleLink: "#ad8301", helpers.ANSIRoleCode: "#66800b",
			helpers.ANSIRoleString: "#66800b", helpers.ANSIRoleKeyword: "#8b7ec8", helpers.ANSIRoleNumber: "#bc5215",
			helpers.ANSIRoleComment: "#878580", helpers.ANSIRoleType: "#24837b", helpers.ANSIRoleFunction: "#4385be",
			helpers.ANSIRoleProperty: "#ad8301", helpers.ANSIRoleConstant: "#ad8301", helpers.ANSIRoleSearch: "#100f0f",
			helpers.ANSIRoleVisualSelection: "#cecdc3", helpers.ANSIRoleActiveTab: "#100f0f", helpers.ANSIRoleSelection: "#cecdc3",
		},
	},
}
