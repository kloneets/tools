package notes

import (
	"testing"

	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
)

func TestMultiCursorAddNextAndTypeTogether(t *testing.T) {
	settings.Init()
	ed := &Editor{Text: "foo foo foo", Cursor: 1, Mode: ModeInsert}
	if !EnterMultiCursor(ed) {
		t.Fatal("EnterMultiCursor() = false")
	}
	if len(ed.MultiCursorLengths) != 1 || ed.MultiCursorLengths[0] != 3 || len(ed.MultiCursorStarts) != 1 || ed.MultiCursorStarts[0] != 0 {
		t.Fatalf("initial cursors = %v lengths %v", ed.MultiCursorStarts, ed.MultiCursorLengths)
	}
	if !AddNextMultiCursor(ed) || !AddNextMultiCursor(ed) || AddNextMultiCursor(ed) {
		t.Fatal("AddNextMultiCursor() did not add two occurrences and stop")
	}
	if !handleMultiCursorInsert(ed, Key{Name: "X", Rune: 'X'}) {
		t.Fatal("multi-cursor typing was not handled")
	}
	if ed.Text != "X X X" {
		t.Fatalf("text after mirrored type = %q", ed.Text)
	}
	if !handleMultiCursorInsert(ed, Key{Name: "Y", Rune: 'Y'}) || ed.Text != "XY XY XY" {
		t.Fatalf("second mirrored type = %q", ed.Text)
	}
	if !handleMultiCursorInsert(ed, Key{Name: "backspace"}) || ed.Text != "X X X" {
		t.Fatalf("mirrored backspace = %q", ed.Text)
	}
	ExitMultiCursor(ed)
	if ed.MultiCursorActive || ed.Cursor != 1 {
		t.Fatalf("exit state: active=%v cursor=%d", ed.MultiCursorActive, ed.Cursor)
	}
}

func TestMultiCursorUndoAndTUIKeyRouting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Init()
	settings.Inst().NotesApp.VimMode = true
	ed := &Editor{Text: "word word", Cursor: 0, Mode: ModeNormal}
	w := &Workspace{Tabs: []*Editor{ed, {Text: "other", Mode: ModeNormal}}, CurrentTab: 0}
	for _, key := range []Key{{Name: ":", Rune: ':'}, {Name: "m", Rune: 'm'}, {Name: "c", Rune: 'c'}, {Name: "enter"}} {
		if !w.HandleKey(key) {
			t.Fatalf("command key %+v was not handled", key)
		}
	}
	if !ed.MultiCursorActive || len(ed.MultiCursorStarts) != 1 {
		t.Fatalf(":mc state active=%v positions=%v", ed.MultiCursorActive, ed.MultiCursorStarts)
	}
	if !w.HandleKey(Key{Name: "n", Rune: 'n', Alt: true}) || len(ed.MultiCursorStarts) != 2 {
		t.Fatalf("Alt+n did not add cursor: %v", ed.MultiCursorStarts)
	}
	if got := len(visualHighlightSpans(ed)); got != 2 {
		t.Fatalf("rendered cursor highlights = %d, want 2", got)
	}
	if !w.HandleKey(Key{Name: "x", Rune: 'x'}) || ed.Text != "x x" {
		t.Fatalf("routed mirrored insert = %q", ed.Text)
	}
	if len(ed.UndoStack) != 1 {
		t.Fatalf("mirrored insert created %d undo entries, want one", len(ed.UndoStack))
	}
	ExitMultiCursor(ed)
	applyUndo(ed)
	if ed.Text != "word word" {
		t.Fatalf("single undo restored %q", ed.Text)
	}
	if !EnterMultiCursor(ed) {
		t.Fatal("could not re-enter multi-cursor mode")
	}
	if !w.setCurrentTab(1) || ed.MultiCursorActive {
		t.Fatal("switching notes did not clear secondary cursors")
	}
}

func TestMultiCursorUsesRuneOffsetsAndDelete(t *testing.T) {
	settings.Init()
	ed := &Editor{Text: "猫猫 猫猫", Cursor: 0, Mode: ModeInsert}
	if !EnterMultiCursor(ed) || !AddNextMultiCursor(ed) {
		t.Fatal("could not establish Unicode cursors")
	}
	if ed.MultiCursorStarts[1] != 3 {
		t.Fatalf("second rune offset = %d, want 3", ed.MultiCursorStarts[1])
	}
	if !handleMultiCursorInsert(ed, Key{Name: "delete"}) || ed.Text != " " {
		t.Fatalf("mirrored delete = %q", ed.Text)
	}
}

func TestMultiCursorVisualSelectionAndCommand(t *testing.T) {
	settings.Init()
	ed := &Editor{Text: "alpha alpha", Cursor: 2, Mode: ModeVisual, SelectionMode: vimSelectionChar, SelectionMark: 0, SelectionCursor: 4}
	if !EnterMultiCursor(ed) || ed.MultiCursorLengths[0] != 5 {
		t.Fatal("visual selection was not used")
	}
	if !AddNextMultiCursor(ed) || ed.MultiCursorStarts[1] != 6 {
		t.Fatalf("next selected occurrence = %v", ed.MultiCursorStarts)
	}
}

func TestMultiCursorMovementKeepsQueryForNextOccurrence(t *testing.T) {
	settings.Init()
	ed := &Editor{Text: "foo foo foo", Cursor: 1, Mode: ModeInsert}
	if !EnterMultiCursor(ed) || !AddNextMultiCursor(ed) {
		t.Fatal("could not add initial occurrence")
	}
	if !handleMultiCursorInsert(ed, Key{Name: "right"}) {
		t.Fatal("cursor movement was not handled")
	}
	if ed.MultiCursorLengths[0] != 0 || ed.MultiCursorLengths[1] != 0 {
		t.Fatalf("movement did not collapse ranges: %v", ed.MultiCursorLengths)
	}
	if !AddNextMultiCursor(ed) || len(ed.MultiCursorStarts) != 3 || ed.MultiCursorLengths[2] != 3 {
		t.Fatalf("could not add after movement: starts=%v lengths=%v", ed.MultiCursorStarts, ed.MultiCursorLengths)
	}
	if !handleMultiCursorInsert(ed, Key{Name: "x", Rune: 'x'}) || ed.Text != "foox foox x" {
		t.Fatalf("edit after movement/add = %q", ed.Text)
	}
}

func TestMultiCursorLineSelectionAndBlockSelection(t *testing.T) {
	settings.Init()
	ed := &Editor{Text: "first\nfirst\nsecond", Cursor: 0, Mode: ModeVisual, SelectionMode: vimSelectionLine, SelectionMark: 0, SelectionCursor: 0}
	if !EnterMultiCursor(ed) || ed.MultiCursorQuery != "first\n" {
		t.Fatalf("line selection query = %q", ed.MultiCursorQuery)
	}
	block := &Editor{Text: "abcd\nx\n猫yz", Cursor: 2, Mode: ModeVisual, SelectionMode: vimSelectionBlock, SelectionMark: 2, SelectionCursor: 9}
	if !EnterMultiCursor(block) || len(block.MultiCursorStarts) != 3 || block.MultiCursorStarts[1] != 6 || !block.MultiCursorPadPending || !block.MultiCursorBlockMode {
		t.Fatalf("block cursors = %v, pad pending = %v, block mode = %v", block.MultiCursorStarts, block.MultiCursorPadPending, block.MultiCursorBlockMode)
	}
}

func TestBlockUpperIInsertsAtEachLeftEdgeAndPadsLazily(t *testing.T) {
	settings.Init()
	ed := &Editor{
		Text:            "abcd\nx\n猫yz",
		Cursor:          2,
		Mode:            ModeVisual,
		SelectionMode:   vimSelectionBlock,
		SelectionMark:   2,
		SelectionCursor: 9,
	}
	if !handleVisualMode(&Workspace{}, ed, Key{Name: "I", Rune: 'I', Shift: true}) {
		t.Fatal("block I was not handled")
	}
	if !ed.MultiCursorActive || ed.Text != "abcd\nx\n猫yz" || !ed.MultiCursorPadPending {
		t.Fatalf("enter block insert: active=%v text=%q pad pending=%v", ed.MultiCursorActive, ed.Text, ed.MultiCursorPadPending)
	}
	if AddNextMultiCursor(ed) || len(ed.MultiCursorStarts) != 3 {
		t.Fatal("block row cursors should not use occurrence-add navigation")
	}
	if !handleMultiCursorInsert(ed, Key{Name: "!", Rune: '!'}) {
		t.Fatal("mirrored insert was not handled")
	}
	if got, want := ed.Text, "ab!cd\nx !\n猫y!z"; got != want {
		t.Fatalf("block insert text = %q, want %q", got, want)
	}
	if !handleMultiCursorInsert(ed, Key{Name: "backspace"}) || ed.Text != "abcd\nx \n猫yz" {
		t.Fatalf("mirrored backspace text = %q", ed.Text)
	}
	if !handleMultiCursorInsert(ed, Key{Name: "esc"}) || ed.MultiCursorActive || ed.Mode != ModeNormal {
		t.Fatal("Escape did not exit block insertion mode")
	}
}

func TestBlockUpperIWithoutEditsDoesNotPadShortRows(t *testing.T) {
	settings.Init()
	ed := &Editor{
		Text:            "abcd\nx\n猫yz",
		Cursor:          2,
		Mode:            ModeVisual,
		SelectionMode:   vimSelectionBlock,
		SelectionMark:   2,
		SelectionCursor: 9,
	}
	if !handleVisualMode(&Workspace{}, ed, Key{Name: "I", Rune: 'I'}) {
		t.Fatal("block I was not handled")
	}
	if !handleMultiCursorInsert(ed, Key{Name: "esc"}) {
		t.Fatal("Escape was not handled")
	}
	if ed.Text != "abcd\nx\n猫yz" {
		t.Fatalf("no-op block insert changed text to %q", ed.Text)
	}
}

func TestVisualBlockDWDeletesWordsPerRowAndKeepsBlockRegister(t *testing.T) {
	settings.Init()
	restore := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restore()
	ed := &Editor{
		Text:            "aa apple pie\nc\nbb berry tart",
		Mode:            ModeVisual,
		SelectionMode:   vimSelectionBlock,
		SelectionMark:   3,
		SelectionCursor: 18,
	}
	for _, key := range []Key{{Name: "d", Rune: 'd'}, {Name: "w", Rune: 'w'}} {
		if !handleVisualMode(&Workspace{}, ed, key) {
			t.Fatalf("block operator key %q was not handled", key.Name)
		}
	}
	if got, want := ed.Text, "aa  pie\nc\nbb  tart"; got != want {
		t.Fatalf("block dw text = %q, want %q", got, want)
	}
	if ed.Register.Kind != vimRegisterBlock || len(ed.Register.Lines) != 3 || ed.Register.Lines[0] != "apple" || ed.Register.Lines[1] != "" || ed.Register.Lines[2] != "berry" {
		t.Fatalf("block dw register = %#v", ed.Register)
	}
	if ed.Mode != ModeNormal || ed.SelectionMode != vimSelectionNone {
		t.Fatalf("block dw mode/selection = %q/%q", ed.Mode, ed.SelectionMode)
	}
	if !applyUndo(ed) || ed.Text != "aa apple pie\nc\nbb berry tart" {
		t.Fatalf("undo block dw restored %q", ed.Text)
	}
}

func TestVisualBlockChangeWordEntersMirroredInsert(t *testing.T) {
	settings.Init()
	ed := &Editor{
		Text:            "foo abc\nbar xyz",
		Mode:            ModeVisual,
		SelectionMode:   vimSelectionBlock,
		SelectionMark:   0,
		SelectionCursor: 8,
	}
	for _, key := range []Key{{Name: "c", Rune: 'c'}, {Name: "w", Rune: 'w'}} {
		if !handleVisualMode(&Workspace{}, ed, key) {
			t.Fatalf("block change key %q was not handled", key.Name)
		}
	}
	if !ed.MultiCursorActive || ed.Mode != ModeInsert || ed.Text != " abc\n xyz" || len(ed.MultiCursorStarts) != 2 {
		t.Fatalf("block cw state: active=%v mode=%q text=%q cursors=%v", ed.MultiCursorActive, ed.Mode, ed.Text, ed.MultiCursorStarts)
	}
	if !handleMultiCursorInsert(ed, Key{Name: "z", Rune: 'z'}) || ed.Text != "z abc\nz xyz" {
		t.Fatalf("mirrored block change text = %q", ed.Text)
	}
	ExitMultiCursor(ed)
	if !applyUndo(ed) || ed.Text != " abc\n xyz" {
		t.Fatalf("first undo after cw = %q, want the deleted-text state", ed.Text)
	}
	if !applyUndo(ed) || ed.Text != "foo abc\nbar xyz" {
		t.Fatalf("second undo after cw = %q, want original text", ed.Text)
	}
}

func TestVisualBlockDWUsesRuneOffsetsForUnicode(t *testing.T) {
	settings.Init()
	restore := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restore()
	ed := &Editor{Text: "猫猫 hello\n犬犬 world", Mode: ModeVisual, SelectionMode: vimSelectionBlock, SelectionMark: 3, SelectionCursor: 12}
	for _, key := range []Key{{Name: "d", Rune: 'd'}, {Name: "w", Rune: 'w'}} {
		if !handleVisualMode(&Workspace{}, ed, key) {
			t.Fatalf("block operator key %q was not handled", key.Name)
		}
	}
	if got, want := ed.Text, "猫猫 \n犬犬 "; got != want {
		t.Fatalf("Unicode block dw text = %q, want %q", got, want)
	}
}

func TestVisualBlockDOperatorsStayWithinEachSelectedRow(t *testing.T) {
	restore := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restore()
	tests := []struct {
		name, motion, text, want string
		mark, cursor             int
	}{
		{
			name:   "delete to line end across short middle row",
			motion: "$", text: "aa apple\nc\nbb berry", want: "aa \nc\nbb ",
			mark: 3, cursor: 14,
		},
		{
			name:   "delete backward without crossing short middle row",
			motion: "b", text: "aa apple\nc\nbb berry", want: "aa pple\nc\nbb erry",
			mark: 15, cursor: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := &Editor{Text: tt.text, Mode: ModeVisual, SelectionMode: vimSelectionBlock, SelectionMark: tt.mark, SelectionCursor: tt.cursor}
			for _, key := range []Key{{Name: "d", Rune: 'd'}, {Name: tt.motion, Rune: []rune(tt.motion)[0]}} {
				if !handleVisualMode(&Workspace{}, ed, key) {
					t.Fatalf("block operator key %q was not handled", key.Name)
				}
			}
			if ed.Text != tt.want {
				t.Fatalf("text = %q, want %q", ed.Text, tt.want)
			}
		})
	}
}

func TestVisualBlockChangeToLineEndEntersCursorsAfterDeletion(t *testing.T) {
	restore := helpers.SetClipboardWriterForTesting(func(string) error { return nil })
	defer restore()
	ed := &Editor{Text: "foo abc\nx\nbar xyz", Mode: ModeVisual, SelectionMode: vimSelectionBlock, SelectionMark: 4, SelectionCursor: 14}
	for _, key := range []Key{{Name: "c", Rune: 'c'}, {Name: "$", Rune: '$'}} {
		if !handleVisualMode(&Workspace{}, ed, key) {
			t.Fatalf("block change key %q was not handled", key.Name)
		}
	}
	if ed.Text != "foo \nx\nbar " || !ed.MultiCursorActive || len(ed.MultiCursorStarts) != 2 {
		t.Fatalf("block c$ state: text=%q active=%v cursors=%v", ed.Text, ed.MultiCursorActive, ed.MultiCursorStarts)
	}
	if !handleMultiCursorInsert(ed, Key{Name: "z", Rune: 'z'}) || ed.Text != "foo z\nx\nbar z" {
		t.Fatalf("mirrored c$ insertion = %q", ed.Text)
	}
}

func TestVisualBlockPendingOperatorCancelsAndXDeletesRectangle(t *testing.T) {
	settings.Init()
	ed := &Editor{Text: "abcd\nwxyz", Mode: ModeVisual, SelectionMode: vimSelectionBlock, SelectionMark: 1, SelectionCursor: 7}
	if !handleVisualMode(&Workspace{}, ed, Key{Name: "d", Rune: 'd'}) || ed.Text != "abcd\nwxyz" || ed.PendingOp != "d" {
		t.Fatal("block d should wait for a motion")
	}
	if !handleVisualMode(&Workspace{}, ed, Key{Name: "esc"}) || ed.PendingOp != "" || ed.SelectionMode != vimSelectionNone {
		t.Fatal("Escape should cancel the pending operator and selection")
	}
	ed.Mode = ModeVisual
	ed.SelectionMode = vimSelectionBlock
	ed.SelectionMark = 1
	ed.SelectionCursor = 7
	if !handleVisualMode(&Workspace{}, ed, Key{Name: "x", Rune: 'x'}) || ed.Text != "ad\nwz" {
		t.Fatalf("x rectangle delete text = %q", ed.Text)
	}
}
