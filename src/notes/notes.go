package notes

import (
	"log"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/ui"
)

type Note struct {
	F          *gtk.Frame
	note       *gtk.TextView
	saveButton *gtk.Button
}

func GenerateUI() *Note {
	n := Note{}

	n.note = gtk.NewTextView()
	n.note.SetWrapMode(gtk.WrapWord)
	n.note.SetSizeRequest(300, 200)
	n.note.Buffer().SetText(getNoteText())
	n.saveButton = gtk.NewButtonFromIconName("document-save")
	n.saveButton.SetTooltipText("Save")
	n.saveButton.ConnectClicked(n.save)
	n.saveButton.SetMarginStart(ui.DefaultBoxPadding)
	n.saveButton.SetMarginEnd(ui.DefaultBoxPadding)

	mainArea := ui.MainArea()
	mainArea.Append(n.note)
	mainArea.Append(n.saveButton)

	n.F = ui.Frame("Notes:")
	n.F.SetChild(mainArea)

	return &n
}

func getNoteText() string {
	c, err := os.ReadFile(fileName())
	if err != nil {
		log.Fatal(err)
		return ""
	}

	return string(c)
}

func fileName() string {
	dirname, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	return filepath.Join(
		dirname,
		helpers.AppConfigMainDir,
		helpers.AppConfigAppDir,
		"notes.txt")
}

func (n *Note) save() {

	file := fileName()

	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()

	buffer := n.note.Buffer()

	if _, err := f.WriteString(
		buffer.Text(buffer.StartIter(),
			buffer.EndIter(),
			true)); err != nil {
		log.Println(err)
		return
	}

	helpers.StatusBarInst().UpdateStatusBar("Notes saved to: " + file)
}
