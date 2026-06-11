//go:build windows

package clusterconnect

import "os"

func signalInterrupt(process *os.Process) error {
	return process.Kill()
}
