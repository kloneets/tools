package password

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/kloneets/tools/src/settings"
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
	appSettings := settings.Inst()
	p := PasswordGenerator{}

	p.generate = gtk.NewButton()
	p.generate.SetLabel("Generate")
	p.generate.ConnectClicked(p.genPassword)
	p.generate.SetMarginStart(ui.DefaultBoxPadding)
	p.generate.SetMarginEnd(ui.DefaultBoxPadding)
	p.letters = gtk.NewCheckButtonWithLabel("letters")
	p.letters.SetActive(appSettings.PasswordApp.Letters)
	p.letters.ConnectToggled(p.saveSettings)
	p.numbers = gtk.NewCheckButtonWithLabel("numbers")
	p.numbers.SetActive(appSettings.PasswordApp.Numbers)
	p.numbers.ConnectToggled(p.saveSettings)
	p.specialSymbols = gtk.NewCheckButtonWithLabel("special symbols")
	p.specialSymbols.SetActive(appSettings.PasswordApp.SpecialSymbols)
	p.specialSymbols.ConnectToggled(p.saveSettings)
	p.password = gtk.NewEntry()
	p.password.SetMarginStart(ui.DefaultBoxPadding)
	p.password.SetMarginEnd(ui.DefaultBoxPadding)
	p.symbolCount = gtk.NewEntry()
	p.symbolCount.SetText(fmt.Sprint(appSettings.PasswordApp.SymbolCount))

	countContainer := ui.FieldWrapper(gtk.NewLabel("Symbol count:"), ui.DefaultBoxPadding)
	countContainer.Append(p.symbolCount)

	mainArea := ui.MainArea()
	mainArea.Append(countContainer)
	mainArea.Append(p.letters)
	mainArea.Append(p.numbers)
	mainArea.Append(p.specialSymbols)
	mainArea.Append(p.generate)
	mainArea.Append(p.password)

	p.F = ui.Frame("Generate password:")
	p.F.SetChild(mainArea)

	return &p
}

func (p *PasswordGenerator) genPassword() {
	sCount, err := strconv.Atoi(p.symbolCount.Text())
	if err != nil {
		sCount = defaultPasswordLength
	}

	charPool := buildCharPool(p.letters.Active(), p.numbers.Active(), p.specialSymbols.Active())
	if len(charPool) == 0 {
		p.password.SetText("")
		return
	}

	source := rand.NewSource(time.Now().UnixNano())
	p.password.SetText(generatePassword(charPool, sCount, rand.New(source)))
	p.saveSettings()
}

func (p *PasswordGenerator) saveSettings() {
	s := settings.Inst()
	s.PasswordApp.Letters = p.letters.Active()
	s.PasswordApp.Numbers = p.numbers.Active()
	s.PasswordApp.SpecialSymbols = p.specialSymbols.Active()
	sc, err := strconv.Atoi(p.symbolCount.Text())
	if err != nil {
		sc = defaultPasswordLength
	}
	s.PasswordApp.SymbolCount = sc

	settings.SaveSettings()
}

func buildCharPool(includeLetters bool, includeNumbers bool, includeSpecial bool) string {
	charPool := ""

	if includeLetters {
		for ch := 'a'; ch <= 'z'; ch++ {
			charPool += fmt.Sprintf("%c", ch)
		}

		charPool += strings.ToUpper(charPool)
	}

	if includeSpecial {
		charPool += "`~!@#$%^&*()_+\\|/{}[]'\";:><.,"
	}

	if includeNumbers {
		charPool += "0123456789"
	}

	return charPool
}

func generatePassword(charPool string, symbolCount int, random *rand.Rand) string {
	var builder strings.Builder
	builder.Grow(symbolCount)

	for i := 0; i < symbolCount; i++ {
		builder.WriteByte(charPool[random.Intn(len(charPool))])
	}

	return builder.String()
}
