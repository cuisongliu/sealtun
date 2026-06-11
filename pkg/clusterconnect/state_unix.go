//go:build !windows

package clusterconnect

import (
	"os"
	"syscall"
)

func signalInterrupt(process *os.Process) error {
	return process.Signal(syscall.SIGINT)
}
