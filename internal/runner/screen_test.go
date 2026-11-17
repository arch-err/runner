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

func TestVimScrollKeys(t *testing.T) {
	s := &screen{out: &bytes.Buffer{}, width: 40, height: 10, active: true}
	s.reset()
	defer s.terminal.Close()
	for n := 0; n < 40; n++ {
		fmt.Fprintf(s, "line %02d\n", n)
	}
	key := func(code rune, mod uv.KeyMod) { s.input(uv.KeyPressEvent{Code: code, Mod: mod}) }
	s.top = 10
	key('k', 0)
	if s.top != 9 {
		t.Fatal("k did not scroll up one line")
	}
	key('j', 0)
	if s.top != 10 {
		t.Fatal("j did not scroll down one line")
	}
	key('u', uv.ModCtrl)
	if s.top != 6 {
		t.Fatal("Ctrl-U did not scroll up half a page")
	}
	key('d', uv.ModCtrl)
	if s.top != 10 {
		t.Fatal("Ctrl-D did not scroll down half a page")
	}
	key('g', 0)
	if s.top != 10 {
		t.Fatal("single g scrolled")
	}
	key('g', 0)
	if s.top != 0 {
		t.Fatal("gg did not go to the top")
	}
	key('G', 0)
	if s.top != -1 {
		t.Fatal("G did not follow the bottom")
	}
	key('g', 0)
	key('k', 0)
	before := s.top
	key('g', 0)
	if s.top != before {
		t.Fatal("unrelated key did not cancel pending g")
	}
	s.begin()
	key('g', 0)
	if s.top != -1 {
		t.Fatal("rerun retained pending g")
	}
}
