package cmd

import (
	"fmt"

	tunnelprotocol "github.com/labring/sealtun/pkg/protocol"
)

type tunnelEndpointDisplay struct {
	Kind        string
	Host        string
	Port        int32
	Command     string
	URL         string
	ControlHost string
}

func endpointDisplay(protocol, host, sealosHost string, publicPort int32) tunnelEndpointDisplay {
	host = valueOr(host, sealosHost)
	if protocol == tunnelprotocol.SSH {
		display := tunnelEndpointDisplay{
			Kind:        "ssh",
			Host:        host,
			Port:        publicPort,
			ControlHost: valueOr(sealosHost, host),
		}
		if host != "" && host != "-" && publicPort != 0 {
			display.Command = fmt.Sprintf("ssh <user>@%s -p %d", host, publicPort)
		}
		return display
	}
	display := tunnelEndpointDisplay{
		Kind:        "https",
		Host:        host,
		ControlHost: valueOr(sealosHost, host),
	}
	if host != "" && host != "-" {
		display.URL = "https://" + host
	}
	return display
}

func endpointLabel(protocol, host, sealosHost string, publicPort int32) string {
	display := endpointDisplay(protocol, host, sealosHost, publicPort)
	if display.Kind == "ssh" {
		if display.Command != "" {
			return display.Command
		}
		if display.Host != "" && display.Host != "-" {
			return display.Host
		}
		return "-"
	}
	if display.URL != "" {
		return display.URL
	}
	return "-"
}
