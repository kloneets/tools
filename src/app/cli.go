package app

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/notes"
)

func RunCLI(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, bool) {
	if len(args) == 0 || args[0] != "ol" {
		return 0, false
	}
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	text, err := readOpenLinksInput(args[1:], stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1, true
	}
	links := notes.CollectSupportedLinks(text)
	for _, link := range links {
		helpers.OpenURI(link)
	}
	fmt.Fprintf(stdout, "opened %d link(s)\n", len(links))
	return 0, true
}

func readOpenLinksInput(paths []string, stdin io.Reader) (string, error) {
	if len(paths) == 0 {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	var b strings.Builder
	for _, path := range paths {
		var data []byte
		var err error
		if path == "-" {
			data, err = io.ReadAll(stdin)
		} else {
			data, err = os.ReadFile(path)
		}
		if err != nil {
			return "", err
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.Write(data)
	}
	return b.String(), nil
}
