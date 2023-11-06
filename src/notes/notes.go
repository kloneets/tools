package notes

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/ui"
)

type Note struct {
	F             *gtk.Frame
	note          *gtk.TextView
	WaitingToSave bool
}

var saveCounter int

func GenerateUI() *Note {
	saveCounter = 0
	n := Note{
		WaitingToSave: false,
	}

	n.note = gtk.NewTextView()
	n.note.SetWrapMode(gtk.WrapWord)
	n.note.SetSizeRequest(300, 200)
	n.note.Buffer().SetText(getNoteText())
	n.note.Buffer().ConnectChanged(n.autoSave)
	n.note.SetMarginStart(ui.DefaultBoxPadding)
	n.note.SetMarginEnd(ui.DefaultBoxPadding)
	n.note.SetMarginTop(ui.DefaultBoxPadding)
	n.note.SetMarginBottom(ui.DefaultBoxPadding)
	scrollW := gtk.NewScrolledWindow()
	scrollW.SetMaxContentHeight(400)
	scrollW.SetMinContentHeight(300)
	scrollW.SetChild(n.note)
	mainArea := ui.MainArea()
	mainArea.Append(scrollW)

	n.F = ui.Frame("Notes:")
	n.F.SetChild(mainArea)

	return &n
}

func (n *Note) autoSave() {
	if !n.WaitingToSave {
		n.WaitingToSave = true
		time.AfterFunc(3*time.Second, n.save)
	}
}

func getNoteText() string {
	c, err := os.ReadFile(fileName())
	if err != nil {
		log.Println("Didn't find notes file:", err)
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

	if err := os.Truncate(file, 0); err != nil {
		log.Println("Notes read file error: ", err)
	}

	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println("Notes read file error: ", err)
		return
	}
	defer f.Close()

	buffer := n.note.Buffer()

	t := buffer.Text(buffer.StartIter(), buffer.EndIter(), true)
	log.Println(t)
	if _, err := f.WriteString(t); err != nil {
		log.Println(err)
		return
	}
	saveCounter++
	helpers.StatusBarInst().UpdateStatusBar("Notes saved to: " + file + ", " + fmt.Sprint(saveCounter))
	n.WaitingToSave = false
	f.Close()
}
