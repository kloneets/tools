package pages

import (
	"fmt"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/ui"
)

type KokoPages struct {
	F           *gtk.Frame
	Box         *gtk.Box
	fbEntry     *gtk.Entry
	fbReadEntry *gtk.Entry
	sbEntry     *gtk.Entry
	calcButton  *gtk.Button
	resLabel    *gtk.Label
}

func PageUi() *KokoPages {
	p := KokoPages{}

	p.fbEntry = gtk.NewEntry()
	p.fbReadEntry = gtk.NewEntry()
	p.sbEntry = gtk.NewEntry()
	p.resLabel = gtk.NewLabel("")
	p.calcButton = gtk.NewButtonFromIconName("input-dialpad")
	p.calcButton.SetTooltipText("Calculate")
	p.calcButton.ConnectClicked(p.Calculate)

	hBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
	hBox.Append(gtk.NewLabel("Pages in book : Pages read : Pages in other edition"))

	hBox2 := gtk.NewBox(gtk.OrientationHorizontal, 1)
	hBox2.Append(p.fbEntry)
	hBox2.Append(gtk.NewLabel(":"))
	hBox2.Append(p.sbEntry)
	hBox2.Append(gtk.NewLabel(":"))
	hBox2.Append(p.fbReadEntry)
	hBox2.Append(gtk.NewLabel(" "))
	hBox2.Append(p.calcButton)

	hBox3 := gtk.NewBox(gtk.OrientationHorizontal, 0)
	hBox3.Append(gtk.NewLabel("Result: "))
	hBox3.Append(p.resLabel)

	p.Box = ui.MainArea()
	p.Box.Append(hBox)
	p.Box.Append(hBox2)
	p.Box.Append(hBox3)
	p.Box.SetMarginStart(ui.DefaultBoxPadding)
	p.Box.SetMarginEnd(ui.DefaultBoxPadding)

	p.F = ui.Frame("Pages: ")
	p.F.SetChild(p.Box)

	return &p
}

func (p *KokoPages) Calculate() {
	readPages, err := strconv.Atoi(p.fbReadEntry.Text())
	if err != nil {
		readPages = 1
	}

	maxFirstPages, err := strconv.Atoi(p.fbEntry.Text())
	if err != nil {
		maxFirstPages = 1
	}

	maxSecondPages, err := strconv.Atoi(p.sbEntry.Text())
	if err != nil {
		maxSecondPages = 1
	}

	res := readPages * maxSecondPages / maxFirstPages
	p.resLabel.SetText(fmt.Sprint(res))
}
