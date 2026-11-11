package runner

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func stopSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
func pokeSignals() []os.Signal { return nil }

type processGroup struct {
	job windows.Handle
	pid uint32
}

func prepareProcess(cmd *exec.Cmd) (processGroup, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return processGroup{}, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	if err != nil {
		windows.CloseHandle(job)
		return processGroup{}, err
	}
	// Assign the child to its job before it can spawn descendants.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP}
	return processGroup{job: job}, nil
}

func (g *processGroup) attach(cmd *exec.Cmd) error {
	g.pid = uint32(cmd.Process.Pid)
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, g.pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(g.job, process); err != nil {
		return err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err := windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != g.pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		_, err = windows.ResumeThread(thread)
		windows.CloseHandle(thread)
		return err
	}
	return errors.New("could not find the suspended command's primary thread")
}

func (g *processGroup) terminate() {
	_ = windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, g.pid)
}

func (g *processGroup) alive() bool {
	// The accounting structure consists of four int64 times and four uint32 counters.
	var accounting struct {
		Times                                                            [4]int64
		PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
	}
	err := windows.QueryInformationJobObject(g.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
	return err != nil || accounting.ActiveProcesses != 0
}

func (g *processGroup) close() {
	if g.job != 0 {
		_ = windows.TerminateJobObject(g.job, 1)
		windows.CloseHandle(g.job)
		g.job = 0
	}
}

func lockFile(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}
