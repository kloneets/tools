package notes

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
	"github.com/shuLhan/share/lib/hunspell"
)

type SpellDictionary struct {
	Code    string
	Name    string
	Package string
	License string
}

var spellCatalog = []SpellDictionary{
	{Code: "en", Name: "English", Package: "dictionary-en", License: "MIT AND BSD"},
	{Code: "ru", Name: "Russian", Package: "dictionary-ru", License: "BSD-3-Clause"},
	{Code: "lv", Name: "Latvian", Package: "dictionary-lv", License: "LGPL-2.1"},
	{Code: "de", Name: "German", Package: "dictionary-de", License: "GPL-2.0 OR GPL-3.0"},
	{Code: "fr", Name: "French", Package: "dictionary-fr", License: "MPL-2.0"},
	{Code: "es", Name: "Spanish", Package: "dictionary-es", License: "GPL-3.0 OR LGPL-3.0 OR MPL-1.1"},
	{Code: "lt", Name: "Lithuanian", Package: "dictionary-lt", License: "BSD-3-Clause"},
	{Code: "uk", Name: "Ukrainian", Package: "dictionary-uk", License: "GPL-3.0"},
}

var (
	spellCacheMu     sync.Mutex
	spellCacheKey    string
	spellCache       *spellService
	spellRefreshMu   sync.Mutex
	spellRefreshHook func()
	spellStatusMu    sync.Mutex
	spellStatusCache = map[string]SpellDictionaryLoadStatus{}
	spellHTTPGet     = http.Get
	spellLookPath    = exec.LookPath
	spellRunCommand  = runSpellCommand
	spellDownloadURL = func(pkg string, file string) string {
		return fmt.Sprintf("https://unpkg.com/%s/%s", pkg, file)
	}
)

type SpellDictionaryLoadStatus struct {
	Code      string
	Installed bool
	Loaded    bool
	Fallback  bool
	Backend   string
	Action    string
	Error     string
}

type spellService struct {
	mu              sync.Mutex
	dictionaries    []*hunspell.Spell
	native          []nativeSpellDictionary
	custom          map[string]struct{}
	fallback        map[string]struct{}
	checked         map[string]bool
	suggestionCache map[string][]string
	loadErrors      []error
}

type nativeSpellDictionary struct {
	backend string
	command string
	affPath string
	dicPath string
}

type spellToken struct {
	word  string
	start int
	end   int
}

func SpellCatalog() []SpellDictionary {
	out := make([]SpellDictionary, len(spellCatalog))
	copy(out, spellCatalog)
	return out
}

func SpellDictionaryByCode(code string) (SpellDictionary, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	for _, dict := range spellCatalog {
		if dict.Code == code {
			return dict, true
		}
	}
	return SpellDictionary{}, false
}

func SpellDictionaryInstalled(code string) bool {
	_, aff, dic, _ := spellDictionaryPaths(code)
	if aff == "" || dic == "" {
		return false
	}
	_, affErr := os.Stat(aff)
	_, dicErr := os.Stat(dic)
	return affErr == nil && dicErr == nil
}

func SpellDictionaryStatus(code string) SpellDictionaryLoadStatus {
	code = strings.ToLower(strings.TrimSpace(code))
	status := SpellDictionaryLoadStatus{Code: code}
	_, affPath, dicPath, _ := spellDictionaryPaths(code)
	if affPath == "" || dicPath == "" || !SpellDictionaryInstalled(code) {
		return status
	}
	status.Installed = true
	cacheKey := code + "|" + nativeSpellAvailabilitySignature() + "|" + fileSignature(affPath) + "|" + fileSignature(dicPath)
	spellStatusMu.Lock()
	if cached, ok := spellStatusCache[cacheKey]; ok {
		spellStatusMu.Unlock()
		return cached
	}
	spellStatusMu.Unlock()

	if native, err := availableNativeSpellDictionary(affPath, dicPath); err == nil {
		status.Loaded = true
		status.Backend = native.backend
	} else if dict, err := loadSpellDictionary(affPath, dicPath); err == nil {
		status.Loaded = dict != nil
		if status.Loaded {
			status.Backend = "go-hunspell"
		}
	} else {
		status.Error = err.Error()
		if words, fallbackErr := loadFallbackDictionaryWords(dicPath); fallbackErr == nil && len(words) > 0 {
			status.Fallback = true
			status.Backend = "fallback"
			status.Action = nativeSpellInstallAction()
		} else if fallbackErr != nil {
			status.Error = fmt.Sprintf("%s; fallback: %s", status.Error, fallbackErr.Error())
		} else {
			status.Error = status.Error + "; fallback: no words"
		}
	}

	spellStatusMu.Lock()
	spellStatusCache[cacheKey] = status
	spellStatusMu.Unlock()
	return status
}

func InstallSpellDictionary(code string) error {
	dict, ok := SpellDictionaryByCode(code)
	if !ok {
		return fmt.Errorf("unknown spell dictionary: %s", code)
	}
	dir, affPath, dicPath, licensePath := spellDictionaryPaths(dict.Code)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	required := map[string]string{
		"index.aff": affPath,
		"index.dic": dicPath,
	}
	for file, target := range required {
		if err := downloadSpellFile(spellDownloadURL(dict.Package, file), target); err != nil {
			return err
		}
	}
	if err := downloadSpellFile(spellDownloadURL(dict.Package, "license"), licensePath); err != nil {
		_ = os.WriteFile(licensePath, []byte(dict.License+"\n"), 0o644)
	}
	addEnabledSpellDictionary(dict.Code)
	invalidateSpellCache()
	return nil
}

func EnableSpellDictionary(code string) bool {
	before := strings.Join(settings.Inst().NotesApp.SpellDictionaries, "\x00")
	addEnabledSpellDictionary(code)
	after := strings.Join(settings.Inst().NotesApp.SpellDictionaries, "\x00")
	if before != after {
		invalidateSpellCache()
		return true
	}
	return false
}

func AddCustomWord(word string) (bool, error) {
	word = normalizeSpellWord(word)
	if word == "" {
		return false, fmt.Errorf("no word under cursor")
	}
	words, err := loadCustomWords()
	if err != nil {
		return false, err
	}
	key := strings.ToLower(word)
	if _, ok := words[key]; ok {
		return false, nil
	}
	path := CustomSpellWordsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, word); err != nil {
		return false, err
	}
	updateSpellCacheAfterCustomWord(word)
	return true, nil
}

func CustomSpellWordsPath() string {
	return filepath.Join(spellRootDir(), "custom.txt")
}

func spellHighlightSpans(text string) []markdownSpan {
	return spellHighlightSpansForEditor(nil, text)
}

func spellHighlightSpansForEditor(ed *Editor, text string) []markdownSpan {
	if !settings.Inst().NotesApp.SpellCheckEnabled {
		return nil
	}
	if ed != nil && ed.SpellCacheText == text {
		return filterSpellSpansForActiveWord(ed, ed.SpellCacheSpans)
	}
	if reused, ok := reuseSpellSpansForActiveWordEdit(ed, text); ok {
		return reused
	}
	service, err := currentSpellService()
	if err != nil || service == nil || !service.ready() {
		return nil
	}
	ignored := spellIgnoredSpans(text)
	tokens := spellTokens(text)
	words := spellTokenWords(tokens)
	if ed != nil && len(service.native) > 0 && len(service.pendingNativeWords(words)) > 0 {
		scheduleEditorSpellCheck(ed, text, service, words, ignored, tokens)
		return nil
	}
	service.checkWords(words)
	spans := spellSpansFromCheckedService(ed, text, service, ignored, tokens)
	if ed != nil {
		ed.SpellCacheText = text
		ed.SpellCacheSpans = append(ed.SpellCacheSpans[:0], spans...)
	}
	return spans
}

func scheduleEditorSpellCheck(ed *Editor, text string, service *spellService, words []string, ignored []markdownSpan, tokens []spellToken) {
	if ed == nil || service == nil {
		return
	}
	if ed.SpellAsyncRunning && ed.SpellAsyncText == text {
		return
	}
	ed.SpellAsyncText = text
	ed.SpellAsyncRunning = true
	ed.SpellCacheText = text
	ed.SpellCacheSpans = nil
	go func() {
		service.checkWords(words)
		spans := spellSpansFromCheckedService(ed, text, service, ignored, tokens)
		if ed.Text == text {
			ed.SpellCacheText = text
			ed.SpellCacheSpans = append(ed.SpellCacheSpans[:0], spans...)
		}
		ed.SpellAsyncRunning = false
		triggerSpellRefresh()
	}()
}

func spellSpansFromCheckedService(ed *Editor, text string, service *spellService, ignored []markdownSpan, tokens []spellToken) []markdownSpan {
	spans := make([]markdownSpan, 0)
	for _, token := range tokens {
		if spellRangeOverlaps(token.start, token.end, ignored) {
			continue
		}
		if spellTokenIsActiveInsertWord(ed, token) {
			continue
		}
		if service.correct(token.word) {
			continue
		}
		spans = append(spans, markdownSpan{Tag: tagSpellError, Start: token.start, End: token.end})
	}
	return spans
}

func SetSpellRefreshHook(hook func()) {
	spellRefreshMu.Lock()
	spellRefreshHook = hook
	spellRefreshMu.Unlock()
}

func triggerSpellRefresh() {
	spellRefreshMu.Lock()
	hook := spellRefreshHook
	spellRefreshMu.Unlock()
	if hook != nil {
		hook()
	}
}

func spellTokenIsActiveInsertWord(ed *Editor, token spellToken) bool {
	start, end, ok := spellActiveWordRange(ed)
	if !ok {
		return false
	}
	return token.start < end && token.end > start
}

func spellActiveWordRange(ed *Editor) (int, int, bool) {
	if ed == nil || ed.Mode != ModeInsert {
		return 0, 0, false
	}
	runes := []rune(ed.Text)
	if len(runes) == 0 {
		return 0, 0, false
	}
	cursor := vimClampOffset(ed.Text, ed.Cursor)
	anchor := -1
	if cursor > 0 && cursor-1 < len(runes) && isSpellWordRune(runes[cursor-1]) {
		anchor = cursor - 1
	} else if cursor < len(runes) && isSpellWordRune(runes[cursor]) {
		anchor = cursor
	}
	if anchor < 0 {
		return 0, 0, false
	}
	start := anchor
	for start > 0 && isSpellWordRune(runes[start-1]) {
		start--
	}
	end := anchor + 1
	for end < len(runes) && isSpellWordRune(runes[end]) {
		end++
	}
	return start, end, true
}

func filterSpellSpansForActiveWord(ed *Editor, spans []markdownSpan) []markdownSpan {
	start, end, ok := spellActiveWordRange(ed)
	if !ok {
		return spans
	}
	return filterSpellSpansOutsideRange(spans, start, end)
}

func filterSpellSpansOutsideRange(spans []markdownSpan, start int, end int) []markdownSpan {
	if len(spans) == 0 {
		return nil
	}
	out := make([]markdownSpan, 0, len(spans))
	for _, span := range spans {
		if span.Start < end && span.End > start {
			continue
		}
		out = append(out, span)
	}
	return out
}

func reuseSpellSpansForActiveWordEdit(ed *Editor, text string) ([]markdownSpan, bool) {
	if ed == nil || ed.SpellCacheText == "" {
		return nil, false
	}
	activeStart, activeEnd, ok := spellActiveWordRange(ed)
	if !ok {
		return nil, false
	}
	current := []rune(text)
	cached := []rune(ed.SpellCacheText)
	if activeStart > len(current) || activeEnd > len(current) || activeStart > len(cached) {
		return nil, false
	}
	delta := len(current) - len(cached)
	cachedEnd := activeEnd - delta
	if cachedEnd < activeStart || cachedEnd > len(cached) {
		return nil, false
	}
	if string(current[:activeStart]) != string(cached[:activeStart]) {
		return nil, false
	}
	if string(current[activeEnd:]) != string(cached[cachedEnd:]) {
		return nil, false
	}
	return shiftSpellSpansOutsideRange(ed.SpellCacheSpans, activeStart, cachedEnd, delta), true
}

func shiftSpellSpansOutsideRange(spans []markdownSpan, start int, end int, delta int) []markdownSpan {
	if len(spans) == 0 {
		return nil
	}
	out := make([]markdownSpan, 0, len(spans))
	for _, span := range spans {
		if span.Start < end && span.End > start {
			continue
		}
		if span.Start >= end {
			span.Start += delta
			span.End += delta
		}
		out = append(out, span)
	}
	return out
}

func WordAtOffsetForSpell(text string, offset int) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	if offset >= len(runes) {
		offset = len(runes) - 1
	}
	if offset < 0 {
		offset = 0
	}
	if !isSpellWordRune(runes[offset]) && offset > 0 && isSpellWordRune(runes[offset-1]) {
		offset--
	}
	if !isSpellWordRune(runes[offset]) {
		return ""
	}
	start := offset
	for start > 0 && isSpellWordRune(runes[start-1]) {
		start--
	}
	end := offset + 1
	for end < len(runes) && isSpellWordRune(runes[end]) {
		end++
	}
	return normalizeSpellWord(string(runes[start:end]))
}

func addEnabledSpellDictionary(code string) {
	cfg := settings.Inst()
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return
	}
	for _, existing := range cfg.NotesApp.SpellDictionaries {
		if existing == code {
			return
		}
	}
	cfg.NotesApp.SpellDictionaries = append(cfg.NotesApp.SpellDictionaries, code)
	sort.Strings(cfg.NotesApp.SpellDictionaries)
}

func downloadSpellFile(url string, target string) error {
	resp, err := spellHTTPGet(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	tmp := target + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

func currentSpellService() (*spellService, error) {
	codes := append([]string(nil), settings.Inst().NotesApp.SpellDictionaries...)
	sort.Strings(codes)
	key := spellCacheSignature(codes)
	spellCacheMu.Lock()
	defer spellCacheMu.Unlock()
	if spellCache != nil && spellCacheKey == key {
		return spellCache, nil
	}
	service := &spellService{
		custom:          map[string]struct{}{},
		fallback:        map[string]struct{}{},
		checked:         map[string]bool{},
		suggestionCache: map[string][]string{},
	}
	custom, err := loadCustomWords()
	if err != nil {
		return nil, err
	}
	service.custom = custom
	for _, code := range codes {
		if !SpellDictionaryInstalled(code) {
			continue
		}
		_, affPath, dicPath, _ := spellDictionaryPaths(code)
		if native, err := availableNativeSpellDictionary(affPath, dicPath); err == nil {
			service.native = append(service.native, native)
			continue
		}
		dict, err := loadSpellDictionary(affPath, dicPath)
		if err != nil {
			service.loadErrors = append(service.loadErrors, fmt.Errorf("%s: %w", code, err))
			if words, fallbackErr := loadFallbackDictionaryWords(dicPath); fallbackErr == nil {
				for word := range words {
					service.fallback[word] = struct{}{}
				}
			} else {
				service.loadErrors = append(service.loadErrors, fmt.Errorf("%s fallback: %w", code, fallbackErr))
			}
			continue
		}
		service.dictionaries = append(service.dictionaries, dict)
	}
	spellCache = service
	spellCacheKey = key
	return service, nil
}

func spellCacheSignature(codes []string) string {
	return strings.Join(codes, ",") + "|" + nativeSpellAvailabilitySignature() + "|" + spellFilesSignature(codes) + "|" + customFileSignature()
}

func updateSpellCacheAfterCustomWord(word string) {
	key := strings.ToLower(normalizeSpellWord(word))
	if key == "" {
		return
	}
	spellCacheMu.Lock()
	if spellCache != nil {
		if spellCache.custom == nil {
			spellCache.custom = map[string]struct{}{}
		}
		spellCache.custom[key] = struct{}{}
		if spellCache.checked == nil {
			spellCache.checked = map[string]bool{}
		}
		spellCache.checked[key] = true
		if spellCache.suggestionCache != nil {
			delete(spellCache.suggestionCache, key)
		}
		codes := append([]string(nil), settings.Inst().NotesApp.SpellDictionaries...)
		sort.Strings(codes)
		spellCacheKey = spellCacheSignature(codes)
	}
	spellCacheMu.Unlock()
	if currentNote != nil {
		for _, ed := range currentNote.Tabs {
			if ed == nil {
				continue
			}
			ed.SpellCacheText = ""
			ed.SpellCacheSpans = nil
		}
	}
}

func loadSpellDictionary(affPath string, dicPath string) (*hunspell.Spell, error) {
	return hunspell.Open(affPath, dicPath)
}

func (s *spellService) ready() bool {
	return s != nil && (len(s.native) > 0 || len(s.dictionaries) > 0 || len(s.fallback) > 0 || len(s.custom) > 0)
}

func (s *spellService) correct(word string) bool {
	word = normalizeSpellWord(word)
	if word == "" {
		return true
	}
	s.mu.Lock()
	if _, ok := s.custom[strings.ToLower(word)]; ok {
		s.mu.Unlock()
		return true
	}
	if correct, ok := s.checked[strings.ToLower(word)]; ok {
		s.mu.Unlock()
		return correct
	}
	if _, ok := s.fallback[strings.ToLower(word)]; ok {
		s.mu.Unlock()
		return true
	}
	s.mu.Unlock()
	for _, dict := range s.dictionaries {
		if dict.Spell(word) != nil || dict.Spell(strings.ToLower(word)) != nil {
			return true
		}
	}
	return false
}

func (s *spellService) checkWords(words []string) {
	if s == nil || len(s.native) == 0 || len(words) == 0 {
		return
	}
	pending := s.pendingNativeWords(words)
	if len(pending) == 0 {
		return
	}
	for _, dict := range s.native {
		misspelled, err := dict.misspelledWords(pending)
		if err != nil {
			s.loadErrors = append(s.loadErrors, fmt.Errorf("%s native check: %w", dict.backend, err))
			continue
		}
		s.markNativeWordsChecked(pending, misspelled)
	}
}

func (s *spellService) pendingNativeWords(words []string) []string {
	if s == nil || len(s.native) == 0 || len(words) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := make([]string, 0, len(words))
	seen := make(map[string]struct{}, len(words))
	for _, word := range words {
		word = normalizeSpellWord(word)
		key := strings.ToLower(word)
		if word == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if _, ok := s.custom[key]; ok {
			s.checked[key] = true
			continue
		}
		if _, ok := s.fallback[key]; ok {
			s.checked[key] = true
			continue
		}
		if _, ok := s.checked[key]; ok {
			continue
		}
		pending = append(pending, word)
	}
	return pending
}

func (s *spellService) markNativeWordsChecked(words []string, misspelled map[string]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, word := range words {
		key := strings.ToLower(normalizeSpellWord(word))
		if key == "" {
			continue
		}
		if _, known := s.checked[key]; known && s.checked[key] {
			continue
		}
		_, wrong := misspelled[key]
		s.checked[key] = !wrong
	}
}

func (s *spellService) suggestions(word string) ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("spell service unavailable")
	}
	word = normalizeSpellWord(word)
	if !shouldCheckSpellWord(word) {
		return nil, nil
	}
	key := strings.ToLower(word)
	if _, ok := s.custom[key]; ok {
		return nil, nil
	}
	if suggestions, ok := s.suggestionCache[key]; ok {
		return append([]string(nil), suggestions...), nil
	}
	if len(s.native) == 0 {
		return nil, fmt.Errorf("native suggestions unavailable")
	}
	merged := make([]string, 0, 8)
	seen := map[string]struct{}{}
	for _, dict := range s.native {
		suggestions, err := dict.suggestions(word)
		if err != nil {
			s.loadErrors = append(s.loadErrors, fmt.Errorf("%s native suggestions: %w", dict.backend, err))
			continue
		}
		for _, suggestion := range suggestions {
			suggestion = normalizeSpellWord(suggestion)
			if !shouldCheckSpellWord(suggestion) {
				continue
			}
			lower := strings.ToLower(suggestion)
			if _, ok := seen[lower]; ok {
				continue
			}
			seen[lower] = struct{}{}
			merged = append(merged, suggestion)
		}
	}
	s.suggestionCache[key] = append([]string(nil), merged...)
	return append([]string(nil), merged...), nil
}

func spellTokenWords(tokens []spellToken) []string {
	words := make([]string, 0, len(tokens))
	for _, token := range tokens {
		words = append(words, token.word)
	}
	return words
}

func availableNativeSpellDictionary(affPath string, dicPath string) (nativeSpellDictionary, error) {
	if command, err := spellLookPath("nuspell"); err == nil {
		dict := nativeSpellDictionary{backend: "nuspell", command: command, affPath: affPath, dicPath: dicPath}
		if err := dict.probe(); err == nil {
			return dict, nil
		}
	}
	if command, err := spellLookPath("hunspell"); err == nil {
		dict := nativeSpellDictionary{backend: "hunspell", command: command, affPath: affPath, dicPath: dicPath}
		if err := dict.probe(); err == nil {
			return dict, nil
		}
	}
	return nativeSpellDictionary{}, fmt.Errorf("native checker unavailable; install with: %s", nativeSpellInstallAction())
}

func nativeSpellAvailabilitySignature() string {
	parts := make([]string, 0, 2)
	for _, name := range []string{"nuspell", "hunspell"} {
		path, err := spellLookPath(name)
		if err != nil {
			parts = append(parts, name+":missing")
		} else {
			parts = append(parts, name+":"+path)
		}
	}
	return strings.Join(parts, "|")
}

func (d nativeSpellDictionary) probe() error {
	_, err := d.run([]string{"kokotoolsspellprobe"})
	return err
}

func (d nativeSpellDictionary) misspelledWords(words []string) (map[string]struct{}, error) {
	out, err := d.run(words)
	if err != nil {
		return nil, err
	}
	misspelled := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		for _, word := range nativeMisspelledWords(scanner.Text()) {
			misspelled[strings.ToLower(word)] = struct{}{}
		}
	}
	return misspelled, scanner.Err()
}

func (d nativeSpellDictionary) suggestions(word string) ([]string, error) {
	out, err := d.runSuggestions(word)
	if err != nil {
		return nil, err
	}
	suggestions := make([]string, 0, 8)
	seen := map[string]struct{}{}
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		for _, suggestion := range nativeSuggestionWords(scanner.Text()) {
			key := strings.ToLower(suggestion)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			suggestions = append(suggestions, suggestion)
		}
	}
	return suggestions, scanner.Err()
}

func nativeMisspelledWords(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimSpace(strings.TrimPrefix(line, "Enter some text:"))
	if line == "" || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "INFO:") {
		return nil
	}
	words := make([]string, 0)
	for _, marker := range []string{"& Wrong: ", "# Wrong: "} {
		rest := line
		for {
			idx := strings.Index(rest, marker)
			if idx < 0 {
				break
			}
			rest = rest[idx+len(marker):]
			word := rest
			if end := strings.IndexAny(word, ". \t"); end >= 0 {
				word = word[:end]
			}
			if word = normalizeSpellWord(word); word != "" {
				words = append(words, word)
			}
		}
	}
	if len(words) > 0 {
		return words
	}
	if word := normalizeSpellWord(line); word != "" {
		return []string{word}
	}
	return nil
}

func nativeSuggestionWords(line string) []string {
	line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Enter some text:"))
	if line == "" {
		return nil
	}
	rest := nativeSuggestionList(line)
	if rest == "" {
		return nil
	}
	if cut := strings.Index(rest, "# Wrong:"); cut >= 0 {
		rest = strings.TrimSpace(rest[:cut])
	}
	parts := strings.Split(rest, ",")
	suggestions := make([]string, 0, len(parts))
	for _, part := range parts {
		if word := normalizeSpellWord(part); validNativeSuggestionWord(word) {
			suggestions = append(suggestions, word)
		}
	}
	return suggestions
}

func validNativeSuggestionWord(word string) bool {
	if !shouldCheckSpellWord(word) {
		return false
	}
	return !strings.ContainsAny(word, " \t-")
}

func nativeSuggestionList(line string) string {
	if idx := strings.Index(line, "How about:"); idx >= 0 {
		return strings.TrimSpace(line[idx+len("How about:"):])
	}
	if !strings.HasPrefix(line, "& ") {
		return ""
	}
	if idx := strings.Index(line, ":"); idx >= 0 {
		return strings.TrimSpace(line[idx+1:])
	}
	return ""
}

func (d nativeSpellDictionary) run(words []string) (string, error) {
	input := strings.Join(words, "\n") + "\n"
	switch d.backend {
	case "nuspell":
		return spellRunCommand(d.command, []string{"-d", d.affPath}, input)
	case "hunspell":
		base := strings.TrimSuffix(d.affPath, filepath.Ext(d.affPath))
		return spellRunCommand(d.command, []string{"-d", base, "-l"}, input)
	default:
		return "", fmt.Errorf("unknown native spell backend: %s", d.backend)
	}
}

func (d nativeSpellDictionary) runSuggestions(word string) (string, error) {
	input := word + "\n"
	switch d.backend {
	case "nuspell":
		return spellRunCommand(d.command, []string{"-d", d.affPath}, input)
	case "hunspell":
		base := strings.TrimSuffix(d.affPath, filepath.Ext(d.affPath))
		return spellRunCommand(d.command, []string{"-d", base}, input)
	default:
		return "", fmt.Errorf("unknown native spell backend: %s", d.backend)
	}
}

func runSpellCommand(name string, args []string, input string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func nativeSpellInstallAction() string {
	return "brew install nuspell"
}

func loadFallbackDictionaryWords(path string) (map[string]struct{}, error) {
	words := make(map[string]struct{})
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			if _, err := strconv.Atoi(line); err == nil {
				continue
			}
		}
		word := fallbackDictionaryWord(line)
		if word == "" {
			continue
		}
		words[strings.ToLower(word)] = struct{}{}
	}
	return words, scanner.Err()
}

func fallbackDictionaryWord(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	if idx := strings.IndexAny(line, " \t"); idx >= 0 {
		line = line[:idx]
	}
	if idx := strings.IndexByte(line, '/'); idx >= 0 {
		line = line[:idx]
	}
	return normalizeSpellWord(line)
}

func loadCustomWords() (map[string]struct{}, error) {
	words := make(map[string]struct{})
	f, err := os.Open(CustomSpellWordsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return words, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		word := normalizeSpellWord(scanner.Text())
		if word != "" {
			words[strings.ToLower(word)] = struct{}{}
		}
	}
	return words, scanner.Err()
}

func spellTokens(text string) []spellToken {
	runes := []rune(text)
	tokens := make([]spellToken, 0)
	for i := 0; i < len(runes); {
		if !isSpellWordRune(runes[i]) {
			i++
			continue
		}
		start := i
		for i < len(runes) && isSpellWordRune(runes[i]) {
			i++
		}
		word := string(runes[start:i])
		if shouldCheckSpellWord(word) {
			tokens = append(tokens, spellToken{word: word, start: start, end: i})
		}
	}
	return tokens
}

func shouldCheckSpellWord(word string) bool {
	word = strings.Trim(word, "'’")
	if len([]rune(word)) <= 1 {
		return false
	}
	hasLetter := false
	for _, r := range word {
		if unicode.IsDigit(r) {
			return false
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	return hasLetter
}

func isSpellWordRune(r rune) bool {
	return unicode.IsLetter(r) || r == '\'' || r == '’' || r == '-'
}

func normalizeSpellWord(word string) string {
	return strings.Trim(strings.TrimSpace(word), "'’-.")
}

func spellIgnoredSpans(text string) []markdownSpan {
	spans := editorRenderSpans(text, settings.Inst().NotesApp.TabSpaces)
	ignored := make([]markdownSpan, 0)
	for _, span := range spans {
		switch span.Tag {
		case tagCode, tagCodeBlock, tagCodeKeyword, tagCodeString, tagCodeComment, tagCodeNumber, tagCodeType, tagCodeFunction, tagCodeProperty, tagCodeConstant:
			ignored = append(ignored, span)
		}
	}
	ignored = append(ignored, urlSpans(text)...)
	ignored = append(ignored, markdownLinkURLSpans(text)...)
	sort.SliceStable(ignored, func(i, j int) bool {
		return ignored[i].Start < ignored[j].Start
	})
	return ignored
}

func markdownLinkURLSpans(text string) []markdownSpan {
	lines := strings.SplitAfter(text, "\n")
	spans := make([]markdownSpan, 0)
	offset := 0
	for _, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\n")
		for i := 0; i < len(line); {
			if line[i] != '[' && !strings.HasPrefix(line[i:], "![") {
				_, size := utf8.DecodeRuneInString(line[i:])
				if size <= 0 {
					size = 1
				}
				i += size
				continue
			}
			prefix := 1
			if strings.HasPrefix(line[i:], "![") {
				prefix = 2
			}
			endLabel := strings.IndexByte(line[i+prefix:], ']')
			if endLabel < 0 {
				i += prefix
				continue
			}
			labelEnd := i + prefix + endLabel
			if labelEnd+1 >= len(line) || line[labelEnd+1] != '(' {
				i = labelEnd + 1
				continue
			}
			endURL := strings.IndexByte(line[labelEnd+2:], ')')
			if endURL < 0 {
				i = labelEnd + 2
				continue
			}
			urlStartByte := labelEnd + 2
			urlEndByte := urlStartByte + endURL
			spans = append(spans, markdownSpan{
				Tag:   tagCode,
				Start: offset + runeLen(line[:urlStartByte]),
				End:   offset + runeLen(line[:urlEndByte]),
			})
			i = urlEndByte + 1
		}
		offset += runeLen(rawLine)
	}
	return spans
}

func urlSpans(text string) []markdownSpan {
	runes := []rune(text)
	spans := make([]markdownSpan, 0)
	for i := 0; i < len(runes); {
		if hasURLPrefix(runes, i) {
			start := i
			for i < len(runes) && !unicode.IsSpace(runes[i]) {
				i++
			}
			spans = append(spans, markdownSpan{Tag: tagCode, Start: start, End: i})
			continue
		}
		i++
	}
	return spans
}

func hasURLPrefix(runes []rune, i int) bool {
	for _, prefix := range []string{"http://", "https://", "ftp://", "mailto:"} {
		pr := []rune(prefix)
		if i+len(pr) > len(runes) {
			continue
		}
		if strings.EqualFold(string(runes[i:i+len(pr)]), prefix) {
			return true
		}
	}
	return false
}

func spellRangeOverlaps(start int, end int, spans []markdownSpan) bool {
	for _, span := range spans {
		if span.End <= start {
			continue
		}
		if span.Start >= end {
			return false
		}
		if start < span.End && end > span.Start {
			return true
		}
	}
	return false
}

func spellRootDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "spell")
	}
	return filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "spell")
}

func spellDictionaryPaths(code string) (dir string, aff string, dic string, license string) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return "", "", "", ""
	}
	dir = filepath.Join(spellRootDir(), "dictionaries", code)
	return dir, filepath.Join(dir, "index.aff"), filepath.Join(dir, "index.dic"), filepath.Join(dir, "license")
}

func spellFilesSignature(codes []string) string {
	parts := make([]string, 0, len(codes)*2)
	for _, code := range codes {
		_, aff, dic, _ := spellDictionaryPaths(code)
		parts = append(parts, fileSignature(aff), fileSignature(dic))
	}
	return strings.Join(parts, ";")
}

func customFileSignature() string {
	return fileSignature(CustomSpellWordsPath())
}

func fileSignature(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "missing"
	}
	return fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
}

func invalidateSpellCache() {
	spellCacheMu.Lock()
	spellCache = nil
	spellCacheKey = ""
	spellCacheMu.Unlock()
	spellStatusMu.Lock()
	spellStatusCache = map[string]SpellDictionaryLoadStatus{}
	spellStatusMu.Unlock()
	if currentNote != nil {
		for _, ed := range currentNote.Tabs {
			if ed == nil {
				continue
			}
			ed.SpellCacheText = ""
			ed.SpellCacheSpans = nil
		}
	}
}

func resetSpellTestHooks() {
	spellHTTPGet = http.Get
	spellLookPath = exec.LookPath
	spellRunCommand = runSpellCommand
	SetSpellRefreshHook(nil)
	spellDownloadURL = func(pkg string, file string) string {
		return fmt.Sprintf("https://unpkg.com/%s/%s", pkg, file)
	}
	invalidateSpellCache()
}

func SetSpellHTTPGetForTests(fn func(string) (*http.Response, error)) {
	spellHTTPGet = fn
	invalidateSpellCache()
}

func SetSpellDownloadURLForTests(fn func(string, string) string) {
	spellDownloadURL = fn
}

func SetSpellNativeHooksForTests(lookPath func(string) (string, error), runCommand func(string, []string, string) (string, error)) {
	spellLookPath = lookPath
	spellRunCommand = runCommand
	invalidateSpellCache()
}

func ResetSpellTestHooksForTests() {
	resetSpellTestHooks()
}
