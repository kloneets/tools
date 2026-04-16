package password

import (
	"math/rand"
	"strings"
	"time"

	"github.com/kloneets/tools/src/settings"
)

const defaultPasswordLength = 16

type Model struct {
	Letters        bool
	Numbers        bool
	SpecialSymbols bool
	SymbolCount    int
	Password       string
	Dirty          bool
}

func NewModel() *Model {
	cfg := settings.Inst().PasswordApp
	if cfg.SymbolCount <= 0 {
		cfg.SymbolCount = defaultPasswordLength
	}
	return &Model{
		Letters:        cfg.Letters,
		Numbers:        cfg.Numbers,
		SpecialSymbols: cfg.SpecialSymbols,
		SymbolCount:    cfg.SymbolCount,
	}
}

func (m *Model) Generate() {
	charPool := buildCharPool(m.Letters, m.Numbers, m.SpecialSymbols)
	if len(charPool) == 0 {
		m.Password = ""
		m.save()
		return
	}
	source := rand.NewSource(time.Now().UnixNano())
	m.Password = generatePassword(charPool, m.SymbolCount, rand.New(source))
	m.save()
}

func (m *Model) save() {
	s := settings.Inst()
	s.PasswordApp.Letters = m.Letters
	s.PasswordApp.Numbers = m.Numbers
	s.PasswordApp.SpecialSymbols = m.SpecialSymbols
	if m.SymbolCount <= 0 {
		m.SymbolCount = defaultPasswordLength
	}
	s.PasswordApp.SymbolCount = m.SymbolCount
	m.Dirty = true
}

func (m *Model) Save() {
	if m == nil {
		return
	}
	m.save()
	settings.SaveSettingsLocal()
	m.Dirty = false
}

func buildCharPool(includeLetters bool, includeNumbers bool, includeSpecial bool) string {
	charPool := ""
	if includeLetters {
		for ch := 'a'; ch <= 'z'; ch++ {
			charPool += string(ch)
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
