package pages

import (
	"fmt"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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

	p.Box = gtk.NewBox(gtk.OrientationVertical, 3)
	p.Box.Append(hBox)
	p.Box.Append(hBox2)
	p.Box.Append(hBox3)
	p.Box.SetMarginStart(2)
	p.Box.SetMarginEnd(2)

	p.F = gtk.NewFrame("Pages:")
	p.F.SetMarginTop(2)
	p.F.SetMarginStart(2)
	p.F.SetMarginEnd(2)

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
