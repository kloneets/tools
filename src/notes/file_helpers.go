package notes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func noteRelPath(path string) string {
	rel, err := filepath.Rel(notesDir(), path)
	if err != nil {
		return filepath.Base(path)
	}
	return rel
}

func uniquePathLike(target string, original string, isDir bool) string {
	if target == original {
		return target
	}
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return target
	}
	dir := filepath.Dir(target)
	base := filepath.Base(target)
	ext := ""
	name := base
	if !isDir {
		ext = filepath.Ext(base)
		name = strings.TrimSuffix(base, ext)
	}
	for i := 2; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s %d%s", name, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func replaceRunes(ed *Editor, start int, end int, replacement string) {
	if ed == nil {
		return
	}
	runes := []rune(ed.Text)
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if end > len(runes) {
		end = len(runes)
	}
	ed.Text = string(runes[:start]) + replacement + string(runes[end:])
	ed.Cursor = start + len([]rune(replacement))
}
