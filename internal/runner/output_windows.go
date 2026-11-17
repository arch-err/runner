package runner

import (
	"os"
	"os/exec"
)

func captureOutput(cmd *exec.Cmd, s *screen) (*outputCapture, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	return makeCapture(cmd, s, reader, writer), nil
}
