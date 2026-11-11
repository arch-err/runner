package runner

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsJobStopsDescendants(t *testing.T) {
	t.Setenv("RUNNER_TEST_HELPER", "1")
	dir := t.TempDir()
	pidfile := filepath.Join(dir, "child.pid")
	p, err := startProcess([]string{os.Args[0], "-test.run=^TestHelperProcess$", "--", "tree", pidfile}, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	var pid int
	eventually(t, "child starts inside the job", func() bool {
		data, _ := os.ReadFile(pidfile)
		pid, _ = strconv.Atoi(string(data))
		return pid > 0
	})
	child, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(child)
	p.stop(100 * time.Millisecond)
	status, err := windows.WaitForSingleObject(child, 3000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("descendant survived job cleanup: status %d, %v", status, err)
	}
}
