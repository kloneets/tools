package helpers

import (
	"os/exec"
	"runtime"
)

var uriOpener = defaultOpenURI

func OpenURI(uri string) {
	if uri == "" {
		return
	}
	uriOpener(uri)
}

func SetURIOpenerForTesting(fn func(string)) func() {
	previous := uriOpener
	if fn == nil {
		uriOpener = defaultOpenURI
	} else {
		uriOpener = fn
	}
	return func() {
		uriOpener = previous
	}
}

func defaultOpenURI(uri string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", uri)
	default:
		cmd = exec.Command("xdg-open", uri)
	}
	_ = cmd.Start()
}
