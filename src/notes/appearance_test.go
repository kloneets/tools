package notes

import (
	"testing"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/settings"
)

func TestParseFontSpecUsesFamilyAndSize(t *testing.T) {
	family, size := parseFontSpec("IBM Plex Sans 12", "Cantarell 11")
	if family != "IBM Plex Sans" {
		t.Fatalf("family = %q, want IBM Plex Sans", family)
	}
	if size != 12 {
		t.Fatalf("size = %v, want 12", size)
	}
}

func TestParseFontSpecFallsBackOnInvalidValue(t *testing.T) {
	family, size := parseFontSpec("broken", "Cantarell 11")
	if family != "Cantarell" {
		t.Fatalf("family = %q, want Cantarell", family)
	}
	if size != 11 {
		t.Fatalf("size = %v, want 11", size)
	}
}

func TestNotesThemePaletteProvidesDistinctThemes(t *testing.T) {
	dark := notesThemePalette(previewThemeIDEDark)
	neon := notesThemePalette(previewThemeNeonBurst)
	light := notesThemePalette(previewThemePaperLight)
	mono := notesThemePalette(previewThemeMono)

	if dark.previewBackground == light.previewBackground || dark.codeKeyword == light.codeKeyword {
		t.Fatal("paper-light theme should differ from ide-dark theme")
	}
	if neon.codeKeyword != "#ff4ecd" {
		t.Fatalf("neon code keyword = %q, want #ff4ecd", neon.codeKeyword)
	}
	if neon.codeProperty != "#00e5ff" {
		t.Fatalf("neon code property = %q, want #00e5ff", neon.codeProperty)
	}
	if neon.codeString != "#9dff6b" {
		t.Fatalf("neon code string = %q, want #9dff6b", neon.codeString)
	}
	if neon.codeForeground == neon.codeKeyword || neon.codeForeground == neon.codeProperty || neon.codeForeground == neon.codeString {
		t.Fatal("neon token colors should differ from the base code foreground")
	}
	if neon.previewBackground == dark.previewBackground {
		t.Fatal("neon theme should differ from ide-dark theme")
	}
	if dark.previewBackground != "#1a1b26" {
		t.Fatalf("dark preview background = %q, want #1a1b26", dark.previewBackground)
	}
	if dark.codeKeyword != "#bb9af7" {
		t.Fatalf("dark code keyword = %q, want #bb9af7", dark.codeKeyword)
	}
	if mono.codeKeyword != "#000000" {
		t.Fatalf("mono code keyword = %q, want #000000", mono.codeKeyword)
	}
}

func TestConfigureMarkdownTagsSetsDeterministicPriorityOrder(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	buffer := gtk.NewTextBuffer(nil)
	configureMarkdownTags(buffer, currentAppearance(), true)

	order := markdownTagOrder()
	for idx, name := range order {
		tag := buffer.TagTable().Lookup(name)
		if tag == nil {
			t.Fatalf("missing tag %q", name)
		}
		if got := tag.Priority(); got != idx {
			t.Fatalf("tag %q priority = %d, want %d", name, got, idx)
		}
	}

	if buffer.TagTable().Lookup(tagCodeKeyword).Priority() <= buffer.TagTable().Lookup(tagPreviewBody).Priority() {
		t.Fatal("token tags must outrank preview body tag")
	}
	if buffer.TagTable().Lookup(tagCodeKeyword).Priority() <= buffer.TagTable().Lookup(tagCodeBlock).Priority() {
		t.Fatal("token tags must outrank code block tag")
	}
	if buffer.TagTable().Lookup(tagVisualSelection).Priority() <= buffer.TagTable().Lookup(tagCodeBlock).Priority() {
		t.Fatal("visual selection tag must outrank code block tag")
	}
}

func TestConfigureMarkdownTagsDoesNotIndentCodeBlocks(t *testing.T) {
	requireGTK(t)
	t.Setenv("HOME", t.TempDir())
	settings.Init()

	buffer := gtk.NewTextBuffer(nil)
	configureMarkdownTags(buffer, currentAppearance(), true)

	tag := buffer.TagTable().Lookup(tagCodeBlock)
	if tag == nil {
		t.Fatal("missing code block tag")
	}
	if got, ok := tag.ObjectProperty("left-margin").(int); !ok || got != 0 {
		t.Fatalf("code block left-margin = %#v, want 0", tag.ObjectProperty("left-margin"))
	}
}

func TestCurrentAppearanceUsesMonospaceEditorWhenEnabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.BodyFont = "Cantarell 11"
	settings.Inst().NotesApp.MonospaceFont = "JetBrains Mono 12"
	settings.Inst().NotesApp.EditorMonospace = true

	appearance := currentAppearance()
	if appearance.editorFamily != "JetBrains Mono" || appearance.editorSize != 12 {
		t.Fatalf("editor font = %q/%v, want JetBrains Mono/12", appearance.editorFamily, appearance.editorSize)
	}
	if appearance.previewFamily != "Cantarell" || appearance.previewSize != 11 {
		t.Fatalf("preview font = %q/%v, want Cantarell/11", appearance.previewFamily, appearance.previewSize)
	}
}

func TestCurrentAppearanceUsesEditorFontSizeOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.BodyFont = "Cantarell 11"
	settings.Inst().NotesApp.EditorFontSize = 18

	appearance := currentAppearance()
	if appearance.editorFamily != "Cantarell" || appearance.editorSize != 18 {
		t.Fatalf("editor font = %q/%v, want Cantarell/18", appearance.editorFamily, appearance.editorSize)
	}
	if appearance.previewSize != 11 {
		t.Fatalf("preview size = %v, want 11", appearance.previewSize)
	}
}
