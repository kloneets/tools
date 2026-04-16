package pages

import (
	"fmt"
	"strconv"

	"github.com/kloneets/tools/src/settings"
)

type Model struct {
	FirstBookInput  string
	ReadInput       string
	SecondBookInput string
	Result          string
	Focus           int
	Editing         bool
	Dirty           bool
}

func NewModel() *Model {
	cfg := settings.Inst().PagesApp
	m := &Model{
		FirstBookInput:  fmt.Sprint(cfg.FirstBookPages),
		ReadInput:       fmt.Sprint(cfg.ReadPages),
		SecondBookInput: fmt.Sprint(cfg.SecondBookPages),
	}
	m.Recalculate()
	return m
}

func (m *Model) Recalculate() {
	readPages, maxFirstPages, maxSecondPages := sanitizeInputs(m.ReadInput, m.FirstBookInput, m.SecondBookInput)
	res, resPercents := calculateResult(readPages, maxFirstPages, maxSecondPages)
	m.Result = fmt.Sprintf("%d pages, %d%%", res, resPercents)

	s := settings.Inst()
	s.PagesApp.FirstBookPages = maxFirstPages
	s.PagesApp.SecondBookPages = maxSecondPages
	s.PagesApp.ReadPages = readPages
}

func (m *Model) Move(delta int) {
	m.Focus += delta
	if m.Focus < 0 {
		m.Focus = 2
	}
	if m.Focus > 2 {
		m.Focus = 0
	}
}

func (m *Model) StartEditing() {
	m.Editing = true
}

func (m *Model) StopEditing() {
	m.Editing = false
}

func (m *Model) IsEditing() bool {
	return m != nil && m.Editing
}

func (m *Model) Cursor() (int, int, bool) {
	if m == nil || !m.Editing {
		return 0, 0, false
	}
	row := 2 + m.Focus
	col := len([]rune(m.focusPrefix() + m.focusLabel()))
	col += len([]rune(m.focusedValue()))
	return row, col, true
}

func (m *Model) HandleEditKey(name string, r rune) bool {
	if m == nil {
		return false
	}
	switch name {
	case "esc":
		m.StopEditing()
		return true
	case "enter":
		m.StopEditing()
		m.Recalculate()
		m.Dirty = true
		return true
	case "backspace":
		value := []rune(m.focusedValue())
		if len(value) == 0 {
			return true
		}
		m.setFocusedValue(string(value[:len(value)-1]))
		return true
	}
	if r >= '0' && r <= '9' {
		m.setFocusedValue(m.focusedValue() + string(r))
		m.Dirty = true
		return true
	}
	return false
}

func (m *Model) Save() {
	if m == nil {
		return
	}
	m.Recalculate()
	settings.SaveSettingsLocal()
	m.Dirty = false
}

func (m *Model) focusedValue() string {
	switch m.Focus {
	case 0:
		return m.FirstBookInput
	case 1:
		return m.ReadInput
	default:
		return m.SecondBookInput
	}
}

func (m *Model) focusPrefix() string {
	if m.Editing {
		return "> "
	}
	if m.Focus >= 0 && m.Focus <= 2 {
		return "* "
	}
	return "  "
}

func (m *Model) focusLabel() string {
	switch m.Focus {
	case 0:
		return "first book:  "
	case 1:
		return "read pages:  "
	default:
		return "other book:  "
	}
}

func (m *Model) setFocusedValue(value string) {
	switch m.Focus {
	case 0:
		m.FirstBookInput = value
	case 1:
		m.ReadInput = value
	default:
		m.SecondBookInput = value
	}
}

func sanitizeInputs(readText string, firstText string, secondText string) (int, int, int) {
	readPages, err := strconv.Atoi(readText)
	if err != nil {
		readPages = 1
	}
	maxFirstPages, err := strconv.Atoi(firstText)
	if err != nil || maxFirstPages == 0 {
		maxFirstPages = 1
	}
	maxSecondPages, err := strconv.Atoi(secondText)
	if err != nil {
		maxSecondPages = 1
	}
	return readPages, maxFirstPages, maxSecondPages
}

func calculateResult(readPages int, maxFirstPages int, maxSecondPages int) (int, int) {
	res := readPages * maxSecondPages / maxFirstPages
	resPercents := (readPages * 100) / maxFirstPages
	return res, resPercents
}
