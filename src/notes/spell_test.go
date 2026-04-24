package notes

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
)

func TestSpellHighlightSpansUsesCustomWordsAndIgnoresMarkdownCodeAndURLs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	if _, err := AddCustomWord("known"); err != nil {
		t.Fatal(err)
	}
	text := "known badwrd `codebad` https://bad.example [linkbad](https://urlword.example)"
	spans := spellHighlightSpans(text)
	if !containsSpellSpanFor(text, spans, "badwrd") {
		t.Fatalf("spell spans = %#v, want badwrd highlighted", spans)
	}
	if containsSpellSpanFor(text, spans, "known") {
		t.Fatalf("spell spans = %#v, want known accepted from custom dictionary", spans)
	}
	if containsSpellSpanFor(text, spans, "codebad") {
		t.Fatalf("spell spans = %#v, want inline code ignored", spans)
	}
	if containsSpellSpanFor(text, spans, "bad.example") {
		t.Fatalf("spell spans = %#v, want URL ignored", spans)
	}
	if !containsSpellSpanFor(text, spans, "linkbad") {
		t.Fatalf("spell spans = %#v, want markdown link label checked", spans)
	}
	if containsSpellSpanFor(text, spans, "urlword") {
		t.Fatalf("spell spans = %#v, want markdown link URL ignored", spans)
	}
}

func TestRenderEditorPaneSchedulesNativeSpellCheckAsync(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var startedCount atomic.Int32
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(_ string, _ []string, input string) (string, error) {
		if strings.Contains(input, "kokotoolsspellprobe") {
			return "* kokotoolsspellprobe\n", nil
		}
		if startedCount.Add(1) == 1 {
			close(started)
		}
		<-release
		return "& Wrong: badwrd. How about: bad\n", nil
	})
	ed := &Editor{Text: "known badwrd", Mode: ModeNormal}
	go func() {
		_ = renderEditorPane(ed, 40, 1)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		close(release)
		t.Fatal("renderEditorPane blocked on native spellcheck")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("native spellcheck was not scheduled")
	}
	close(release)
	deadline := time.After(time.Second)
	for {
		if ed.SpellCacheText == ed.Text && containsSpellSpanFor(ed.Text, ed.SpellCacheSpans, "badwrd") {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("spell cache = %q %#v, want async badwrd span", ed.SpellCacheText, ed.SpellCacheSpans)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestAddWordUnderCursorPersistsSharedCustomWord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	w := &Workspace{
		Tabs:       []*Editor{{Text: "KokoTools", Cursor: 2, Mode: ModeNormal}},
		CurrentTab: 0,
	}
	if !w.AddWordUnderCursor() {
		t.Fatal("AddWordUnderCursor() = false, want true")
	}
	data, err := os.ReadFile(CustomSpellWordsPath())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "KokoTools\n") {
		t.Fatalf("custom words = %q, want KokoTools", got)
	}
	if status := w.ActiveEditor().Status; !strings.Contains(status, "added word") {
		t.Fatalf("status = %q, want added word", status)
	}
	got := strings.Join(renderEditorPane(w.ActiveEditor(), 40, 1), "\n")
	if strings.Contains(got, helpers.ANSIRoleSpellError) {
		t.Fatalf("renderEditorPane() = %q, want added word to stop being highlighted", got)
	}
}

func TestAddCustomWordKeepsWarmNativeSpellCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	var probes atomic.Int32
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		if strings.Contains(input, "kokotoolsspellprobe") {
			probes.Add(1)
			return "& Wrong: kokotoolsspellprobe. How about: tool\n", nil
		}
		if strings.Contains(input, "badwrd") {
			return "& Wrong: badwrd. How about: badword\n", nil
		}
		return "* OK\n", nil
	})

	_ = spellHighlightSpans("badwrd")
	if got := probes.Load(); got != 1 {
		t.Fatalf("probe count after warmup = %d, want 1", got)
	}
	added, err := AddCustomWord("badwrd")
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatal("AddCustomWord(badwrd) = false, want true")
	}
	spans := spellHighlightSpans("badwrd")
	if len(spans) != 0 {
		t.Fatalf("spellHighlightSpans() = %#v, want custom word accepted without highlight", spans)
	}
	if got := probes.Load(); got != 1 {
		t.Fatalf("probe count after AddCustomWord = %d, want cache preserved without reprobe", got)
	}
}

func TestNormalModeZGAddsWordUnderCursor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	w := &Workspace{
		Tabs:       []*Editor{{Text: "Jaunsvards", Cursor: 1, Mode: ModeNormal}},
		CurrentTab: 0,
	}
	if !w.HandleKey(Key{Name: "z", Rune: 'z'}) || !w.HandleKey(Key{Name: "g", Rune: 'g'}) {
		t.Fatal("zg should add word under cursor")
	}
	data, err := os.ReadFile(CustomSpellWordsPath())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "Jaunsvards\n") {
		t.Fatalf("custom words = %q, want Jaunsvards", got)
	}
}

func TestInstallSpellDictionaryDownloadsFilesAndEnablesDictionary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	defer resetSpellTestHooks()
	spellDownloadURL = func(pkg string, file string) string {
		return pkg + "/" + file
	}
	spellHTTPGet = func(url string) (*http.Response, error) {
		body := "SET UTF-8\n"
		if strings.HasSuffix(url, "index.dic") {
			body = "1\nknown\n"
		}
		if strings.HasSuffix(url, "license") {
			body = "license\n"
		}
		return &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
	if err := InstallSpellDictionary("en"); err != nil {
		t.Fatal(err)
	}
	if !SpellDictionaryInstalled("en") {
		t.Fatal("SpellDictionaryInstalled(en) = false, want true")
	}
	if got := settings.Inst().NotesApp.SpellDictionaries; len(got) != 1 || got[0] != "en" {
		t.Fatalf("SpellDictionaries = %v, want [en]", got)
	}
	_, aff, dic, license := spellDictionaryPaths("en")
	for _, path := range []string{aff, dic, license} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected downloaded file %s: %v", path, err)
		}
	}
}

func TestRenderEditorPaneUsesSpellErrorRole(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	if _, err := AddCustomWord("known"); err != nil {
		t.Fatal(err)
	}
	ed := &Editor{Text: "known badwrd", Mode: ModeNormal}
	got := strings.Join(renderEditorPane(ed, 40, 1), "\n")
	if !strings.Contains(got, helpers.ANSIRoleSpellError) {
		t.Fatalf("renderEditorPane() = %q, want spell error role", got)
	}
	if strings.Contains(got, "\u0332") {
		t.Fatalf("renderEditorPane() = %q, want spell styling without injected underline runes", got)
	}
}

func TestRenderEditorPaneShowsSpellErrorInsideMarkdownSpan(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	if _, err := AddCustomWord("known"); err != nil {
		t.Fatal(err)
	}
	ed := &Editor{Text: "- known badwrd", Mode: ModeNormal}
	got := strings.Join(renderEditorPane(ed, 40, 1), "\n")
	if !strings.Contains(got, helpers.ANSIRoleSpellError) {
		t.Fatalf("renderEditorPane() = %q, want spell error role inside list span", got)
	}
}

func TestRenderEditorPaneSkipsActiveWordSpellCheckWhileTyping(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	if _, err := AddCustomWord("known"); err != nil {
		t.Fatal(err)
	}
	ed := &Editor{Text: "known badwrd", Cursor: len([]rune("known badwrd")), Mode: ModeInsert}
	got := strings.Join(renderEditorPane(ed, 40, 1), "\n")
	if strings.Contains(got, helpers.ANSIRoleSpellError) {
		t.Fatalf("renderEditorPane() = %q, want active word to stay unchecked while typing", got)
	}

	ed.Text = "known badwrd "
	ed.Cursor = len([]rune(ed.Text))
	got = strings.Join(renderEditorPane(ed, 40, 1), "\n")
	if !strings.Contains(got, helpers.ANSIRoleSpellError) {
		t.Fatalf("renderEditorPane() = %q, want completed word checked after delimiter", got)
	}
}

func TestSpellHighlightUsesFallbackDictionaryWhenHunspellLoadFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	SetSpellNativeHooksForTests(func(string) (string, error) {
		return "", errors.New("missing")
	}, nil)
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "2\nknown/nm\nanother/ABC po:test\n")

	spans := spellHighlightSpans("known badwrd")
	if containsSpellSpanFor("known badwrd", spans, "known") {
		t.Fatalf("spell spans = %#v, want known accepted from fallback dictionary", spans)
	}
	if !containsSpellSpanFor("known badwrd", spans, "badwrd") {
		t.Fatalf("spell spans = %#v, want badwrd highlighted", spans)
	}
}

func TestSpellDictionaryStatusReportsFallbackWhenHunspellLoadFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	defer ResetSpellTestHooksForTests()
	SetSpellNativeHooksForTests(func(string) (string, error) {
		return "", errors.New("missing")
	}, nil)
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")

	status := SpellDictionaryStatus("en")
	if !status.Installed {
		t.Fatal("Installed = false, want true")
	}
	if status.Loaded {
		t.Fatal("Loaded = true, want false")
	}
	if !status.Fallback {
		t.Fatal("Fallback = false, want true")
	}
	if !strings.Contains(status.Error, "unknown affix flag") {
		t.Fatalf("Error = %q, want Hunspell load error", status.Error)
	}
}

func TestSpellDictionaryStatusPrefersNativeNuspell(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		if name == "hunspell" {
			return "/bin/hunspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		if name != "/bin/nuspell" {
			t.Fatalf("command = %q, want nuspell", name)
		}
		if len(args) != 2 || args[0] != "-d" || !strings.HasSuffix(args[1], "index.aff") {
			t.Fatalf("args = %v, want nuspell dictionary args", args)
		}
		return "badwrd\n", nil
	})

	status := SpellDictionaryStatus("en")
	if !status.Loaded || status.Backend != "nuspell" {
		t.Fatalf("status = %#v, want native nuspell loaded", status)
	}
}

func TestSpellDictionaryStatusUsesNativeHunspellWhenNuspellMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "hunspell" {
			return "/bin/hunspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		if name != "/bin/hunspell" {
			t.Fatalf("command = %q, want hunspell", name)
		}
		if len(args) != 3 || args[0] != "-d" || args[2] != "-l" {
			t.Fatalf("args = %v, want hunspell -d <base> -l", args)
		}
		return "badwrd\n", nil
	})

	status := SpellDictionaryStatus("en")
	if !status.Loaded || status.Backend != "hunspell" {
		t.Fatalf("status = %#v, want native hunspell loaded", status)
	}
}

func TestNativeSpellCheckHighlightsOnlyMisspelledWordsAndCachesResults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	runCount := 0
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		runCount++
		if strings.Contains(input, "kokotoolsspellprobe") {
			return "& Wrong: kokotoolsspellprobe. How about: tool\n", nil
		}
		if !strings.Contains(input, "known") || !strings.Contains(input, "badwrd") {
			t.Fatalf("input = %q, want batched words", input)
		}
		return "INFO: Pointed dictionary /tmp/index.aff\nEnter some text: * OK\n& Wrong: badwrd. How about: backward\n", nil
	})

	text := "known badwrd"
	spans := spellHighlightSpans(text)
	if containsSpellSpanFor(text, spans, "known") {
		t.Fatalf("spell spans = %#v, want known accepted by native checker", spans)
	}
	if !containsSpellSpanFor(text, spans, "badwrd") {
		t.Fatalf("spell spans = %#v, want badwrd highlighted", spans)
	}
	firstRunCount := runCount
	_ = spellHighlightSpans(text)
	if runCount != firstRunCount {
		t.Fatalf("native command runs = %d, want cached count %d", runCount, firstRunCount)
	}
}

func TestRenderEditorPaneReusesSpellCacheWhileTypingActiveWord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	runCount := 0
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		runCount++
		if strings.Contains(input, "kokotoolsspellprobe") {
			return "& Wrong: kokotoolsspellprobe. How about: tool\n", nil
		}
		return "INFO: Pointed dictionary /tmp/index.aff\nEnter some text: * OK\n& Wrong: badwrd. How about: backward\n", nil
	})

	ed := &Editor{Text: "known badwrd ", Cursor: len([]rune("known badwrd ")), Mode: ModeInsert}
	_ = strings.Join(renderEditorPane(ed, 40, 1), "\n")
	firstRunCount := runCount

	ed.Text = "known badwrdx"
	ed.Cursor = len([]rune(ed.Text))
	_ = strings.Join(renderEditorPane(ed, 40, 1), "\n")
	if runCount != firstRunCount {
		t.Fatalf("native command runs = %d, want unchanged while typing active word (%d)", runCount, firstRunCount)
	}

	ed.Text += " "
	ed.Cursor = len([]rune(ed.Text))
	_ = strings.Join(renderEditorPane(ed, 40, 1), "\n")
	deadline := time.After(time.Second)
	for runCount <= firstRunCount {
		select {
		case <-deadline:
			t.Fatalf("native command runs = %d, want another check after finishing word", runCount)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestRenderEditorPaneDoesNotReuseStaleSpellSpanPositionsWhileTyping(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		if strings.Contains(input, "kokotoolsspellprobe") {
			return "& Wrong: kokotoolsspellprobe. How about: tool\n", nil
		}
		if strings.Contains(input, "badwrd") {
			return "& Wrong: badwrd. How about: backward\n", nil
		}
		return "* OK\n", nil
	})

	ed := &Editor{Text: "known badwrd ", Cursor: len([]rune("known badwrd ")), Mode: ModeInsert}
	_ = strings.Join(renderEditorPane(ed, 40, 1), "\n")

	ed.Text = "known xbadwrd"
	ed.Cursor = len([]rune("known x"))
	got := strings.Join(renderEditorPane(ed, 40, 1), "\n")
	if strings.Contains(got, helpers.ANSIRoleSpellError) {
		t.Fatalf("renderEditorPane() = %q, want no stale underline while active word is being edited", got)
	}
}

func TestNativeSpellCheckHighlightsTodoStyleMisspellings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en"}
	defer ResetSpellTestHooksForTests()
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	SetSpellNativeHooksForTests(func(name string) (string, error) {
		if name == "nuspell" {
			return "/bin/nuspell", nil
		}
		return "", errors.New("missing")
	}, func(name string, args []string, input string) (string, error) {
		if strings.Contains(input, "kokotoolsspellprobe") {
			return "# Wrong: kokotoolsspellprobe. No suggestions.\n", nil
		}
		if !strings.Contains(input, "collor") || !strings.Contains(input, "abowe") {
			t.Fatalf("input = %q, want TODO-style misspellings", input)
		}
		return strings.Join([]string{
			"INFO: Pointed dictionary /tmp/index.aff",
			"Enter some text: & Wrong: collor. How about: color, collar",
			"* OK",
			"# Wrong: abowe. No suggestions.",
		}, "\n"), nil
	})

	text := "- show theme collor preview in settings\n- delete current line and one line abowe"
	spans := spellHighlightSpans(text)
	if !containsSpellSpanFor(text, spans, "collor") {
		t.Fatalf("spell spans = %#v, want collor highlighted", spans)
	}
	if !containsSpellSpanFor(text, spans, "abowe") {
		t.Fatalf("spell spans = %#v, want abowe highlighted", spans)
	}
}

func TestNativeMisspelledWordsParsesNuspellAndHunspellOutput(t *testing.T) {
	cases := map[string][]string{
		"badwrd":                               {"badwrd"},
		"& Wrong: badwrd. How about: backward": {"badwrd"},
		"Enter some text: & Wrong: badwrd. How about: backward":             {"badwrd"},
		"& Wrong: collor. How about: color, collar":                         {"collor"},
		"# Wrong: chpjrsgdhodi. No suggestions.":                            {"chpjrsgdhodi"},
		"& Wrong: collor. How about: color # Wrong: abowe. No suggestions.": {"collor", "abowe"},
		"* OK": nil,
		"INFO: Pointed dictionary /tmp/index.aff": nil,
		"Enter some text: * OK":                   nil,
		"":                                        nil,
	}
	for input, want := range cases {
		if got := nativeMisspelledWords(input); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("nativeMisspelledWords(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestNativeSuggestionWordsParsesOutput(t *testing.T) {
	cases := map[string][]string{
		"& Wrong: collor. How about: color, collar":                                 {"color", "collar"},
		"Enter some text: & Wrong: collor. How about: color, collar":                {"color", "collar"},
		"# Wrong: chpjrsgdhodi. No suggestions.":                                    nil,
		"& Wrong: collor. How about: color, collar # Wrong: abowe. No suggestions.": {"color", "collar"},
	}
	for input, want := range cases {
		if got := nativeSuggestionWords(input); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("nativeSuggestionWords(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestSpellServiceSkipsBrokenDictionaryAndKeepsFallbackWords(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.SpellCheckEnabled = true
	settings.Inst().NotesApp.SpellDictionaries = []string{"en", "lv"}
	defer ResetSpellTestHooksForTests()
	SetSpellNativeHooksForTests(func(string) (string, error) {
		return "", errors.New("missing")
	}, nil)
	writeSpellDictionaryForTest(t, "en", "SET UTF-8\n", "1\nknown/nm\n")
	writeSpellDictionaryForTest(t, "lv", "SET UTF-8\nPFX A Y 1\nPFX A 0 x .\n", "1\nlatviesu/A po:test\n")

	service, err := currentSpellService()
	if err != nil {
		t.Fatal(err)
	}
	if service == nil || !service.ready() {
		t.Fatal("spell service is not ready")
	}
	if !service.correct("known") {
		t.Fatal("known = incorrect, want accepted from fallback dictionary")
	}
	if !service.correct("latviesu") {
		t.Fatal("latviesu = incorrect, want accepted from remaining dictionary")
	}
	if service.correct("badwrd") {
		t.Fatal("badwrd = correct, want misspelled")
	}
}

func containsSpellSpanFor(text string, spans []markdownSpan, word string) bool {
	idx := strings.Index(text, word)
	if idx < 0 {
		return false
	}
	start := len([]rune(text[:idx]))
	end := start + len([]rune(word))
	for _, span := range spans {
		if span.Tag == tagSpellError && span.Start <= start && span.End >= end {
			return true
		}
	}
	return false
}

func writeSpellDictionaryForTest(t *testing.T, code string, aff string, dic string) {
	t.Helper()
	dir, affPath, dicPath, licensePath := spellDictionaryPaths(code)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(affPath, []byte(aff), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dicPath, []byte(dic), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(licensePath, []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	invalidateSpellCache()
}

func TestCustomSpellWordsPathUsesConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "spell", "custom.txt")
	if got := CustomSpellWordsPath(); got != want {
		t.Fatalf("CustomSpellWordsPath() = %q, want %q", got, want)
	}
}
