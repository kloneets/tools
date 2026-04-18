//go:build darwin || linux

package app

import (
	"bufio"
	"io"
	"strings"

	"github.com/kloneets/tools/src/notes"
	"golang.org/x/sys/unix"
)

type termState struct{ state unix.Termios }

func makeRaw(fd int) (*termState, error) {
	oldState, err := getTermios(fd)
	if err != nil {
		return nil, err
	}
	newState := *oldState
	newState.Iflag &^= unix.ICRNL | unix.INLCR | unix.IXON
	newState.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN
	newState.Cflag &^= unix.CSIZE | unix.PARENB
	newState.Cflag |= unix.CS8
	newState.Oflag &^= unix.OPOST
	newState.Cc[unix.VMIN] = 1
	newState.Cc[unix.VTIME] = 0
	if err := setTermios(fd, &newState); err != nil {
		return nil, err
	}
	return &termState{state: *oldState}, nil
}

func restoreTerminal(fd int, state *termState) error {
	if state == nil {
		return nil
	}
	return setTermios(fd, &state.state)
}

func terminalSize(fd int) (int, int) {
	ws, err := getWinsize(fd)
	if err != nil || ws.Col == 0 || ws.Row == 0 {
		return 120, 36
	}
	return int(ws.Col), int(ws.Row)
}

func isTerminal(fd int) bool {
	_, err := getTermios(fd)
	return err == nil
}

func readKeys(r io.Reader, out chan<- notes.Key) {
	reader := bufio.NewReader(r)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			close(out)
			return
		}
		switch b {
		case 3:
			out <- notes.Key{Ctrl: true, Name: "c"}
		case 13, 10:
			out <- notes.Key{Name: "enter"}
		case 9:
			out <- notes.Key{Name: "tab"}
		case 127:
			out <- notes.Key{Name: "backspace"}
		case 27:
			out <- decodeEscapeSequence(reader)
		default:
			name := string([]byte{b})
			out <- notes.Key{Name: name, Rune: rune(b)}
		}
	}
}

func decodeEscapeSequence(reader *bufio.Reader) notes.Key {
	next, err := reader.ReadByte()
	if err != nil {
		return notes.Key{Name: "esc"}
	}
	if next != '[' && next != 'O' {
		_ = reader.UnreadByte()
		return notes.Key{Name: "esc"}
	}
	var seq []byte
	for {
		b, err := reader.ReadByte()
		if err != nil {
			break
		}
		seq = append(seq, b)
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~' {
			break
		}
		if len(seq) > 8 {
			break
		}
	}
	return parseCSI(seq)
}

func parseCSI(seq []byte) notes.Key {
	if len(seq) == 0 {
		return notes.Key{Name: "esc"}
	}
	last := seq[len(seq)-1]
	params := strings.TrimSuffix(string(seq), string(last))
	switch last {
	case 'A':
		return notes.Key{Name: "up"}
	case 'B':
		return notes.Key{Name: "down"}
	case 'C':
		if strings.HasSuffix(params, "5;") || params == "1;5" || params == "5" {
			return notes.Key{Name: "right", Ctrl: true}
		}
		return notes.Key{Name: "right"}
	case 'D':
		if strings.HasSuffix(params, "5;") || params == "1;5" || params == "5" {
			return notes.Key{Name: "left", Ctrl: true}
		}
		return notes.Key{Name: "left"}
	case 'H':
		return notes.Key{Name: "home"}
	case 'F':
		return notes.Key{Name: "end"}
	case '~':
		switch firstParam(params) {
		case "1", "7":
			return notes.Key{Name: "home"}
		case "4", "8":
			return notes.Key{Name: "end"}
		case "3":
			return notes.Key{Name: "delete"}
		case "5":
			return notes.Key{Name: "pageup"}
		case "6":
			return notes.Key{Name: "pagedown"}
		}
	}
	return notes.Key{Name: "esc"}
}

func firstParam(params string) string {
	if params == "" {
		return ""
	}
	parts := strings.Split(params, ";")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

type winsize struct{ Row, Col, Xpixel, Ypixel uint16 }

func getWinsize(fd int) (*winsize, error) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return nil, err
	}
	return &winsize{
		Row:    ws.Row,
		Col:    ws.Col,
		Xpixel: ws.Xpixel,
		Ypixel: ws.Ypixel,
	}, nil
}

func getTermios(fd int) (*unix.Termios, error) { return unix.IoctlGetTermios(fd, termiosReadRequest) }

func setTermios(fd int, termios *unix.Termios) error {
	return unix.IoctlSetTermios(fd, termiosWriteRequest, termios)
}
