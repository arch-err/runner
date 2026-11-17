package runner

import (
	"io"
	"os"
	"os/exec"
	"time"
)

type outputCapture struct {
	reader, writer *os.File
	done           chan struct{}
}

func makeCapture(cmd *exec.Cmd, s *screen, reader, writer *os.File) *outputCapture {
	cmd.Stdout, cmd.Stderr = writer, writer
	c := &outputCapture{reader, writer, make(chan struct{})}
	go func() { _, _ = io.Copy(s, reader); close(c.done) }()
	return c
}
func (c *outputCapture) close() {
	if c == nil {
		return
	}
	c.writer.Close()
	// A descendant which deliberately escapes supervision must not block exit.
	select {
	case <-c.done:
	case <-time.After(time.Second):
		c.reader.Close()
		<-c.done
	}
	c.reader.Close()
}
