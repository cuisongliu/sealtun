//go:build linux

package clusterconnect

func platformTransparentSupported() (bool, string) {
	return true, "Linux transparent TCP mode is available when run as root with iptables"
}
