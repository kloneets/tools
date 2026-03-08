package helpers

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

func OpenURI(uri string) {
	if uri == "" {
		return
	}
	gtk.ShowURI(nil, uri, 0)
}
