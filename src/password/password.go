package password

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/ui"
)

const (
	defaultPasswordLength = 16
)

type PasswordGenerator struct {
	F              *gtk.Frame
	generate       *gtk.Button
	letters        *gtk.CheckButton
	numbers        *gtk.CheckButton
	specialSymbols *gtk.CheckButton
	password       *gtk.Entry
	symbolCount    *gtk.Entry
}

func GenerateUI() *PasswordGenerator {
	p := PasswordGenerator{}

	p.generate = gtk.NewButton()
	p.generate.SetLabel("Generate")
	p.generate.ConnectClicked(p.genPassword)
	p.letters = gtk.NewCheckButtonWithLabel("letters")
	p.letters.SetActive(true)
	p.numbers = gtk.NewCheckButtonWithLabel("numbers")
	p.numbers.SetActive(true)
	p.specialSymbols = gtk.NewCheckButtonWithLabel("special symbols")
	p.specialSymbols.SetActive(true)
	p.password = gtk.NewEntry()
	p.symbolCount = gtk.NewEntry()
	p.symbolCount.SetText(fmt.Sprint(defaultPasswordLength))

	countContainer := ui.FieldWrapper(gtk.NewLabel("Symbol count:"), ui.DefaultBoxPadding)
	countContainer.Append(p.symbolCount)

	mainField := gtk.NewBox(gtk.OrientationVertical, ui.DefaultMasterPadding)
	mainField.SetMarginBottom(ui.DefaultMasterPadding)
	mainField.Append(countContainer)
	mainField.Append(p.letters)
	mainField.Append(p.numbers)
	mainField.Append(p.specialSymbols)
	mainField.Append(ui.FieldWrapper(p.generate, ui.DefaultBoxPadding))
	mainField.Append(ui.FieldWrapper(p.password, ui.DefaultBoxPadding))

	p.F = ui.Frame("Generate password:")
	p.F.SetChild(mainField)

	return &p
}

func (p *PasswordGenerator) genPassword() {
	newPassword := ""
	charPool := ""
	sCount, err := strconv.Atoi(p.symbolCount.Text())
	if err != nil {
		sCount = defaultPasswordLength
	}

	if p.letters.Active() {
		for ch := 'a'; ch < 'z'; ch++ {
			charPool = charPool + fmt.Sprintf("%c", ch)
		}

		charPool = charPool + strings.ToUpper(charPool)
	}

	if p.specialSymbols.Active() {
		charPool = charPool + "`~!@#$%^&*()_+\\|/{}[]'\";:><.,"
	}

	if p.numbers.Active() {
		charPool = charPool + "0123456789"
	}

	if len(charPool) == 0 {
		p.password.SetText("")
		return
	}

	source := rand.NewSource(time.Now().UnixNano())
	myRandom := rand.New(source)

	for i := 0; i < sCount; i++ {
		newPassword = newPassword + string(charPool[myRandom.Intn(len(charPool))])
	}

	p.password.SetText(newPassword)
}
