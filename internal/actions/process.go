package actions

import (
	"fmt"
	"syscall"
)

func Signal(pid int32, sig syscall.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid: %d", pid)
	}
	if err := syscall.Kill(int(pid), sig); err != nil {
		return fmt.Errorf("signal %d to pid %d: %w", sig, pid, err)
	}
	return nil
}
