package runner

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// The child writes directly into a scrolling region between the two status
// lines. Keeping its real terminal descriptors preserves colour and buffering.
type screen struct {
	out            io.Writer
	header, footer string
	width, height  int
	active         bool
	restore        func()
}

func openScreen(header string) *screen {
	s := &screen{out: os.Stdout, header: header, footer: "waiting"}
	if os.Getenv("TERM") == "dumb" || !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprintln(os.Stderr, header)
		return s
	}
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || height < 4 || width < 2 {
		fmt.Fprintln(os.Stderr, header)
		return s
	}
	restore, err := enableTerminal()
	if err != nil {
		fmt.Fprintln(os.Stderr, header)
		return s
	}
	s.width, s.height, s.active, s.restore = width, height, true, restore
	fmt.Fprint(s.out, "\x1b[?1049h\x1b[?25l")
	s.draw()
	return s
}

func (s *screen) chrome() string {
	return fmt.Sprintf("\x1b[1;1H\x1b[0m\x1b[2K\x1b[2m%s\x1b[0m\x1b[%d;1H\x1b[2K\x1b[2m%s\x1b[0m",
		clipLine(s.header, s.width-1), s.height, clipLine(s.footer, s.width-1))
}

func (s *screen) draw() {
	// Origin mode confines even cursor positioning to the command's region.
	fmt.Fprintf(s.out, "\x1b[?6l\x1b[r\x1b[0m\x1b[2J%s\x1b[2;%dr\x1b[?6h\x1b[1;1H", s.chrome(), s.height-1)
}

func (s *screen) begin() {
	if !s.active {
		return
	}
	s.footer = "running"
	s.draw()
}

func (s *screen) finish(code int, elapsed time.Duration) {
	if !s.active {
		return
	}
	s.footer = fmt.Sprintf("exit %d · %s · waiting", code, elapsed.Round(time.Millisecond))
	s.update()
}

func (s *screen) failed() {
	if !s.active {
		return
	}
	s.footer = "start failed · waiting"
	s.update()
}

func (s *screen) update() {
	// DEC save/restore includes attributes and origin mode, so status updates
	// leave a child's cursor and colour state intact.
	fmt.Fprintf(s.out, "\x1b7\x1b[?6l%s\x1b8", s.chrome())
}

func (s *screen) resize() {
	if !s.active {
		return
	}
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || height < 4 || width < 2 || (width == s.width && height == s.height) {
		return
	}
	oldHeight := s.height
	s.width, s.height = width, height
	var clearOld string
	if oldHeight < height {
		clearOld = fmt.Sprintf("\x1b[%d;1H\x1b[2K", oldHeight)
	}
	fmt.Fprintf(s.out, "\x1b7\x1b[?6l%s\x1b[2;%dr%s\x1b8", clearOld, height-1, s.chrome())
}

func (s *screen) Close() {
	if !s.active {
		return
	}
	fmt.Fprint(s.out, "\x1b[?6l\x1b[r\x1b[0m\x1b[?25h\x1b[?1049l")
	s.restore()
}

func clipLine(text string, width int) string {
	text = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(text)
	runes := []rune(text)
	if len(runes) > width {
		runes = runes[:width]
	}
	return string(runes)
}
