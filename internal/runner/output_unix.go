//go:build linux || darwin || freebsd || openbsd || netbsd

package runner

import (
	"github.com/creack/pty"
	"os/exec"
)

func captureOutput(cmd *exec.Cmd, s *screen) (*outputCapture, error) {
	reader, writer, err := pty.Open()
	if err != nil {
		return nil, err
	}
	resize := func(w, h int) { _ = pty.Setsize(reader, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)}) }
	s.mu.Lock()
	resize(s.width-1, s.height-2)
	s.childResize = resize
	s.mu.Unlock()
	return makeCapture(cmd, s, reader, writer), nil
}
