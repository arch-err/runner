package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"golang.org/x/term"
)

// One owner renders the real terminal; child output is interpreted in a bounded
// virtual terminal so scrolling never loses ANSI colours or progress updates.
type screen struct {
	mu             sync.Mutex
	out            io.Writer
	header, footer string
	width, height  int
	active, dirty  bool
	top            int // -1 follows live output; otherwise an anchored history row.
	terminal       *vt.Emulator
	restore        func()
	childResize    func(int, int)
	pendingG       bool
}

func openScreen(header string) *screen {
	s := &screen{out: os.Stdout, header: header, footer: "waiting", top: -1}
	plain := func() *screen { fmt.Fprintln(os.Stderr, header); return s }
	if os.Getenv("TERM") == "dumb" || !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stderr.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return plain()
	}
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || h < 4 || w < 2 {
		return plain()
	}
	restore, err := enableTerminal()
	if err != nil {
		return plain()
	}
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		restore()
		return plain()
	}
	s.width, s.height, s.active = w, h, true
	s.restore = func() { _ = term.Restore(int(os.Stdin.Fd()), state); restore() }
	s.reset()
	fmt.Fprint(s.out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h\x1b[2J")
	s.render()
	return s
}

func (s *screen) reset() {
	if s.terminal != nil {
		_ = s.terminal.Close()
	}
	s.terminal = vt.NewEmulator(s.width-1, s.height-2)
	s.terminal.SetScrollbackSize(10000)
	s.top = -1
	s.pendingG = false
	s.dirty = true
}

func (s *screen) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.terminal.Write(bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n")))
	s.dirty = true
	return len(data), err
}

func (s *screen) render() {
	if !s.active || !s.dirty {
		return
	}
	var frame strings.Builder
	frame.WriteString("\x1b[?6l\x1b[r\x1b[1;1H\x1b[0m\x1b[2K\x1b[2m")
	frame.WriteString(clipLine(s.header, s.width-1))
	frame.WriteString("\x1b[0m")
	history := s.terminal.ScrollbackLen()
	top := s.top
	if top < 0 {
		top = history
	}
	top = min(top, history)
	for row := 0; row < s.height-2; row++ {
		fmt.Fprintf(&frame, "\x1b[%d;1H\x1b[0m\x1b[2K", row+2)
		index := top + row
		var line uv.Line
		if index < history {
			line = s.terminal.Scrollback().Line(index)
		} else {
			for x := 0; x < s.width-1; x++ {
				cell := s.terminal.CellAt(x, index-history)
				if cell == nil {
					line = append(line, uv.EmptyCell)
				} else {
					line = append(line, *cell)
				}
			}
		}
		if len(line) > s.width-1 {
			line = line[:s.width-1]
		}
		frame.WriteString(line.Render())
	}
	footer := s.footer
	if s.top >= 0 {
		footer = "scrolled · End: follow · " + footer
	}
	fmt.Fprintf(&frame, "\x1b[%d;1H\x1b[0m\x1b[2K\x1b[2m%s\x1b[0m", s.height, clipLine(footer, s.width-1))
	fmt.Fprint(s.out, frame.String())
	s.dirty = false
}

func (s *screen) begin() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	s.reset()
	s.footer = "running"
	fmt.Fprint(s.out, "\x1b[2J")
	s.render()
}
func (s *screen) finish(code int, elapsed time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	s.footer = fmt.Sprintf("exit %d · %s · waiting", code, elapsed.Round(time.Millisecond))
	s.dirty = true
	s.render()
}
func (s *screen) failed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	s.footer = "start failed · waiting"
	s.dirty = true
	s.render()
}
func (s *screen) resize() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err == nil && h >= 4 && w >= 2 && (w != s.width || h != s.height) {
		s.width, s.height = w, h
		s.terminal.Resize(w-1, h-2)
		s.dirty = true
		if s.childResize != nil {
			s.childResize(w-1, h-2)
		}
		fmt.Fprint(s.out, "\x1b[2J")
	}
	s.render()
}
func (s *screen) scroll(rows int) {
	if s.top < 0 {
		s.top = s.terminal.ScrollbackLen()
	}
	s.top = max(0, min(s.terminal.ScrollbackLen(), s.top+rows))
	if rows > 0 && s.top == s.terminal.ScrollbackLen() {
		s.top = -1
	}
	s.dirty = true
}
func (s *screen) input(event uv.Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch event := event.(type) {
	case uv.KeyPressEvent:
		key := event.String()
		previousG := s.pendingG
		s.pendingG = false
		switch key {
		case "ctrl+c", "q":
			return true
		case "pgup":
			s.scroll(-(s.height - 2))
		case "pgdown":
			s.scroll(s.height - 2)
		case "up", "k":
			s.scroll(-1)
		case "down", "j":
			s.scroll(1)
		case "ctrl+u":
			s.scroll(-max(1, (s.height-2)/2))
		case "ctrl+d":
			s.scroll(max(1, (s.height-2)/2))
		case "g":
			if previousG {
				s.top = 0
				s.dirty = true
			} else {
				s.pendingG = true
			}
		case "home":
			s.top = 0
			s.dirty = true
		case "end", "G", "shift+g":
			s.top = -1
			s.dirty = true
		}
	case uv.MouseWheelEvent:
		s.pendingG = false
		if event.Button == uv.MouseWheelUp {
			s.scroll(-3)
		} else if event.Button == uv.MouseWheelDown {
			s.scroll(3)
		}
	}
	s.render()
	return false
}
func (s *screen) events(ctx context.Context) <-chan uv.Event {
	if !s.active {
		return nil
	}
	events := make(chan uv.Event, 32)
	reader := uv.NewTerminalReader(os.Stdin, os.Getenv("TERM"))
	mode := uv.MouseModeClick
	reader.MouseMode = &mode
	go func() { _ = reader.StreamEvents(ctx, events) }()
	return events
}
func (s *screen) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	fmt.Fprint(s.out, "\x1b[?1006l\x1b[?1000l\x1b[?6l\x1b[r\x1b[0m\x1b[?25h\x1b[?1049l")
	s.restore()
	_ = s.terminal.Close()
}
func clipLine(text string, width int) string {
	text = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(text)
	runes := []rune(text)
	if len(runes) > width {
		runes = runes[:width]
	}
	return string(runes)
}
