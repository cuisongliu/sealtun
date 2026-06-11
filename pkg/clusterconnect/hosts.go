package clusterconnect

import "strings"

const (
	hostsBegin = "# BEGIN SEALTUN CONNECT"
	hostsEnd   = "# END SEALTUN CONNECT"
)

func removeHostsBlock(text string) string {
	start := strings.Index(text, hostsBegin)
	if start < 0 {
		return text
	}
	end := strings.Index(text[start:], hostsEnd)
	if end < 0 {
		return text
	}
	end += start + len(hostsEnd)
	if end < len(text) && text[end] == '\n' {
		end++
	}
	return text[:start] + text[end:]
}
