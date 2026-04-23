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
	dictionaries []*hunspell.Spell
	native       []nativeSpellDictionary
	custom       map[string]struct{}
	fallback     map[string]struct{}
	checked      map[string]bool
	loadErrors   []error
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
	invalidateSpellCache()
	return true, nil
}

func CustomSpellWordsPath() string {
	return filepath.Join(spellRootDir(), "custom.txt")
}

func spellHighlightSpans(text string) []markdownSpan {
	if !settings.Inst().NotesApp.SpellCheckEnabled {
		return nil
	}
	service, err := currentSpellService()
	if err != nil || service == nil || !service.ready() {
		return nil
	}
	ignored := spellIgnoredSpans(text)
	tokens := spellTokens(text)
	service.checkWords(spellTokenWords(tokens))
	spans := make([]markdownSpan, 0)
	for _, token := range tokens {
		if spellRangeOverlaps(token.start, token.end, ignored) {
			continue
		}
		if service.correct(token.word) {
			continue
		}
		spans = append(spans, markdownSpan{Tag: tagSpellError, Start: token.start, End: token.end})
	}
	return spans
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
	key := strings.Join(codes, ",") + "|" + nativeSpellAvailabilitySignature() + "|" + spellFilesSignature(codes) + "|" + customFileSignature()
	spellCacheMu.Lock()
	defer spellCacheMu.Unlock()
	if spellCache != nil && spellCacheKey == key {
		return spellCache, nil
	}
	service := &spellService{custom: map[string]struct{}{}, fallback: map[string]struct{}{}, checked: map[string]bool{}}
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
	if _, ok := s.custom[strings.ToLower(word)]; ok {
		return true
	}
	if correct, ok := s.checked[strings.ToLower(word)]; ok {
		return correct
	}
	if _, ok := s.fallback[strings.ToLower(word)]; ok {
		return true
	}
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
	if len(pending) == 0 {
		return
	}
	for _, dict := range s.native {
		misspelled, err := dict.misspelledWords(pending)
		if err != nil {
			s.loadErrors = append(s.loadErrors, fmt.Errorf("%s native check: %w", dict.backend, err))
			continue
		}
		for _, word := range pending {
			key := strings.ToLower(word)
			if _, known := s.checked[key]; known && s.checked[key] {
				continue
			}
			_, wrong := misspelled[key]
			s.checked[key] = !wrong
		}
	}
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
}

func resetSpellTestHooks() {
	spellHTTPGet = http.Get
	spellLookPath = exec.LookPath
	spellRunCommand = runSpellCommand
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
