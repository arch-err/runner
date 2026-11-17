package runner

import (
	"bytes"
	"fmt"
	uv "github.com/charmbracelet/ultraviolet"
	"strings"
	"testing"
)

func TestScrollAndFollow(t *testing.T) {
	output := &bytes.Buffer{}
	s := &screen{out: output, width: 30, height: 6, active: true, restore: func() {}}
	s.reset()
	defer s.terminal.Close()
	for n := 0; n < 30; n++ {
		fmt.Fprintf(s, "\x1b[31mline %02d\x1b[0m\n", n)
	}
	s.input(uv.KeyPressEvent{Code: uv.KeyHome})
	if s.top != 0 || !strings.Contains(output.String(), "line 00") {
		t.Fatal("cannot scroll to oldest output")
	}
	output.Reset()
	s.input(uv.KeyPressEvent{Code: uv.KeyPgDown})
	anchor := s.top
	fmt.Fprint(s, "new output\n")
	s.mu.Lock()
	s.render()
	s.mu.Unlock()
	if s.top != anchor {
		t.Fatal("new output moved an anchored view")
	}
	s.input(uv.MouseWheelEvent{Button: uv.MouseWheelUp})
	if s.top >= anchor {
		t.Fatal("mouse wheel did not scroll")
	}
	output.Reset()
	s.input(uv.KeyPressEvent{Code: uv.KeyEnd})
	if s.top != -1 || !strings.Contains(output.String(), "new output") {
		t.Fatal("End did not follow latest output")
	}
	s.begin()
	if s.terminal.ScrollbackLen() != 0 || s.top != -1 {
		t.Fatal("new run retained old output")
	}
}
