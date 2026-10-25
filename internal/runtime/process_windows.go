//go:build windows

package runtime

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var processJobs sync.Map

func configureProcess(command *exec.Cmd, separate bool) {
	if separate {
		command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	}
}

// Job handles are private to the controller. Controller exit closes them and
// terminates attached descendants. No job breakaway flag is granted.
func attachProcess(command *exec.Cmd, separate bool) (func(), error) {
	if !separate {
		return func() {}, nil
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	err = windows.AssignProcessToJobObject(job, process)
	windows.CloseHandle(process)
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	processJobs.Store(command, job)
	return func() { processJobs.Delete(command); windows.CloseHandle(job) }, nil
}

func terminateProcess(command *exec.Cmd, separate bool) {
	if command.Process == nil {
		return
	}
	if separate && windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(command.Process.Pid)) == nil {
		return
	}
	killProcess(command, separate)
}
func killProcess(command *exec.Cmd, _ bool) {
	if job, ok := processJobs.Load(command); ok {
		_ = windows.TerminateJobObject(job.(windows.Handle), 1)
		return
	}
	if command.Process != nil {
		_ = command.Process.Kill()
	}
}
