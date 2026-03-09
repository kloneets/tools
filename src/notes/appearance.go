package notes

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
	"github.com/kloneets/tools/src/settings"
)

const (
	tagEditorBody      = "md-editor-body"
	tagPreviewBody     = "md-preview-body"
	tagVisualSelection = "md-visual-selection"

	previewThemeIDEDark    = "ide-dark"
	previewThemeNeonBurst  = "neon-burst"
	previewThemePaperLight = "paper-light"
	previewThemeMono       = "mono"
)

type notesAppearance struct {
	editorFamily  string
	editorSize    float64
	previewFamily string
	previewSize   float64
	monoFamily    string
	monoSize      float64
	palette       notesPalette
}

type notesPalette struct {
	previewBackground string
	previewForeground string
	quote             string
	link              string
	codeBackground    string
	codeForeground    string
	codeKeyword       string
	codeString        string
	codeComment       string
	codeNumber        string
	codeType          string
	codeFunction      string
	codeProperty      string
	codeConstant      string
}

func init() {
	settings.RegisterSaveHook(func(*settings.UserSettings) {
		if currentNote != nil {
			currentNote.refreshFromSettings()
		}
	})
}

func currentAppearance() notesAppearance {
	noteSettings := settings.Inst().NotesApp
	bodyFamily, bodySize := parseFontSpec(noteSettings.BodyFont, "Cantarell 11")
	monoFamily, monoSize := parseFontSpec(noteSettings.MonospaceFont, "Noto Sans Mono 11")
	editorFamily := bodyFamily
	editorSize := bodySize
	if noteSettings.EditorMonospace {
		editorFamily = monoFamily
		editorSize = monoSize
	}
	if noteSettings.EditorFontSize > 0 {
		editorSize = float64(noteSettings.EditorFontSize)
	}
	return notesAppearance{
		editorFamily:  editorFamily,
		editorSize:    editorSize,
		previewFamily: bodyFamily,
		previewSize:   bodySize,
		monoFamily:    monoFamily,
		monoSize:      monoSize,
		palette:       notesThemePalette(noteSettings.PreviewTheme),
	}
}

func parseFontSpec(spec string, fallback string) (string, float64) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		spec = fallback
	}
	parts := strings.Fields(spec)
	if len(parts) < 2 {
		return parseFontSpec(fallback, fallback)
	}
	size, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || size <= 0 {
		return parseFontSpec(fallback, fallback)
	}
	return strings.Join(parts[:len(parts)-1], " "), float64(size)
}

func notesThemePalette(theme string) notesPalette {
	switch theme {
	case previewThemeNeonBurst:
		return notesPalette{
			previewBackground: "#0b1020",
			previewForeground: "#f5f7ff",
			quote:             "#ff7edb",
			link:              "#5ce1e6",
			codeBackground:    "#11162b",
			codeForeground:    "#f8fbff",
			codeKeyword:       "#ff4ecd",
			codeString:        "#9dff6b",
			codeComment:       "#7d8fb3",
			codeNumber:        "#ffb224",
			codeType:          "#56c2ff",
			codeFunction:      "#7c4dff",
			codeProperty:      "#00e5ff",
			codeConstant:      "#ffe14d",
		}
	case previewThemePaperLight:
		return notesPalette{
			previewBackground: "#f6f1e8",
			previewForeground: "#2d241f",
			quote:             "#8a6d57",
			link:              "#0b63c8",
			codeBackground:    "#ede3d5",
			codeForeground:    "#2d241f",
			codeKeyword:       "#8f2d56",
			codeString:        "#4f772d",
			codeComment:       "#7d7469",
			codeNumber:        "#b24c00",
			codeType:          "#005f73",
			codeFunction:      "#0f4c81",
			codeProperty:      "#8f5a2a",
			codeConstant:      "#8f2d56",
		}
	case previewThemeMono:
		return notesPalette{
			previewBackground: "#f3f3f3",
			previewForeground: "#171717",
			quote:             "#555555",
			link:              "#111111",
			codeBackground:    "#e9e9e9",
			codeForeground:    "#171717",
			codeKeyword:       "#000000",
			codeString:        "#444444",
			codeComment:       "#707070",
			codeNumber:        "#222222",
			codeType:          "#333333",
			codeFunction:      "#111111",
			codeProperty:      "#222222",
			codeConstant:      "#000000",
		}
	default:
		return notesPalette{
			previewBackground: "#1a1b26",
			previewForeground: "#c0caf5",
			quote:             "#565f89",
			link:              "#7aa2f7",
			codeBackground:    "#24283b",
			codeForeground:    "#c0caf5",
			codeKeyword:       "#bb9af7",
			codeString:        "#9ece6a",
			codeComment:       "#565f89",
			codeNumber:        "#ff9e64",
			codeType:          "#7dcfff",
			codeFunction:      "#7aa2f7",
			codeProperty:      "#2ac3de",
			codeConstant:      "#e0af68",
		}
	}
}

func configureMarkdownTags(buffer *gtk.TextBuffer, appearance notesAppearance, preview bool) {
	table := buffer.TagTable()
	for priority, name := range markdownTagOrder() {
		tag := table.Lookup(name)
		if tag == nil {
			tag = gtk.NewTextTag(name)
			table.Add(tag)
		}
		tag.SetPriority(priority)
		markdownTagConfig(name, appearance, preview)(tag)
	}
}

func markdownTagOrder() []string {
	return []string{
		tagEditorBody,
		tagPreviewBody,
		tagHeading1,
		tagHeading2,
		tagHeading3,
		tagList,
		tagOrdered,
		tagChecklist,
		tagQuote,
		tagBold,
		tagItalic,
		tagCode,
		tagCodeBlock,
		tagLink,
		tagCodeKeyword,
		tagCodeString,
		tagCodeComment,
		tagCodeNumber,
		tagCodeType,
		tagCodeFunction,
		tagCodeProperty,
		tagCodeConstant,
		tagVisualSelection,
	}
}

func markdownTagConfig(name string, appearance notesAppearance, preview bool) func(*gtk.TextTag) {
	baseTag := tagEditorBody
	baseForeground := ""
	if preview {
		baseTag = tagPreviewBody
		baseForeground = appearance.palette.previewForeground
	}

	configs := map[string]func(*gtk.TextTag){
		baseTag: func(tag *gtk.TextTag) {
			family := appearance.editorFamily
			size := appearance.editorSize
			if preview {
				family = appearance.previewFamily
				size = appearance.previewSize
			}
			tag.SetObjectProperty("family", family)
			tag.SetObjectProperty("size-points", size)
			if baseForeground != "" {
				tag.SetObjectProperty("foreground", baseForeground)
			}
		},
		tagVisualSelection: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("background", visualSelectionColor(appearance))
		},
		tagHeading1: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("weight", int(pango.WeightBold))
			tag.SetObjectProperty("scale", 1.35)
			tag.SetObjectProperty("pixels-above-lines", 8)
		},
		tagHeading2: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("weight", int(pango.WeightBold))
			tag.SetObjectProperty("scale", 1.2)
			tag.SetObjectProperty("pixels-above-lines", 6)
		},
		tagHeading3: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("weight", int(pango.WeightSemibold))
			tag.SetObjectProperty("scale", 1.1)
			tag.SetObjectProperty("pixels-above-lines", 4)
		},
		tagList: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("left-margin", 12)
		},
		tagQuote: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("style", int(pango.StyleItalic))
			tag.SetObjectProperty("left-margin", 16)
			tag.SetObjectProperty("foreground", appearance.palette.quote)
		},
		tagOrdered: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("left-margin", 12)
		},
		tagChecklist: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("left-margin", 12)
		},
		tagBold: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("weight", int(pango.WeightBold))
		},
		tagItalic: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("style", int(pango.StyleItalic))
		},
		tagCode: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("family", appearance.monoFamily)
			tag.SetObjectProperty("size-points", appearance.monoSize)
			tag.SetObjectProperty("background", appearance.palette.codeBackground)
			tag.SetObjectProperty("foreground", appearance.palette.codeForeground)
		},
		tagCodeBlock: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("family", appearance.monoFamily)
			tag.SetObjectProperty("size-points", appearance.monoSize)
			tag.SetObjectProperty("background", appearance.palette.codeBackground)
			tag.SetObjectProperty("right-margin", 14)
			tag.SetObjectProperty("pixels-above-lines", 6)
			tag.SetObjectProperty("pixels-below-lines", 6)
			tag.SetObjectProperty("paragraph-background", appearance.palette.codeBackground)
		},
		tagCodeKeyword: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("family", appearance.monoFamily)
			tag.SetObjectProperty("foreground", appearance.palette.codeKeyword)
			tag.SetObjectProperty("weight", int(pango.WeightBold))
		},
		tagCodeString: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("family", appearance.monoFamily)
			tag.SetObjectProperty("foreground", appearance.palette.codeString)
		},
		tagCodeComment: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("family", appearance.monoFamily)
			tag.SetObjectProperty("foreground", appearance.palette.codeComment)
			tag.SetObjectProperty("style", int(pango.StyleItalic))
		},
		tagCodeNumber: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("family", appearance.monoFamily)
			tag.SetObjectProperty("foreground", appearance.palette.codeNumber)
		},
		tagCodeType: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("family", appearance.monoFamily)
			tag.SetObjectProperty("foreground", appearance.palette.codeType)
		},
		tagCodeFunction: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("family", appearance.monoFamily)
			tag.SetObjectProperty("foreground", appearance.palette.codeFunction)
		},
		tagCodeProperty: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("family", appearance.monoFamily)
			tag.SetObjectProperty("foreground", appearance.palette.codeProperty)
		},
		tagCodeConstant: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("family", appearance.monoFamily)
			tag.SetObjectProperty("foreground", appearance.palette.codeConstant)
		},
		tagLink: func(tag *gtk.TextTag) {
			tag.SetObjectProperty("underline", int(pango.UnderlineSingle))
			tag.SetObjectProperty("foreground", appearance.palette.link)
		},
	}

	if config, ok := configs[name]; ok {
		return config
	}
	return func(*gtk.TextTag) {}
}

func visualSelectionColor(appearance notesAppearance) string {
	switch appearance.palette.previewBackground {
	case "#0b1020":
		return "#214a85"
	case "#f6f1e8":
		return "#d9c6a3"
	case "#f3f3f3":
		return "#cfcfcf"
	default:
		return "#33467c"
	}
}

func installNotesCSS(appearance notesAppearance) {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}

	css := gtk.NewCSSProvider()
	css.LoadFromData(fmt.Sprintf(`
#notes-preview,
#notes-preview text {
  background-color: %s;
  color: %s;
}

#notes-editor,
#notes-editor text {
  background-color: transparent;
}
`, appearance.palette.previewBackground, appearance.palette.previewForeground))
	gtk.StyleContextAddProviderForDisplay(display, css, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}
