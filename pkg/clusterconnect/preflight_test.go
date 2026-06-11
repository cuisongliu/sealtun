package clusterconnect

import (
	"context"
	"testing"
)

type fakeReviewer struct {
	allowed map[string]bool
}

func (f fakeReviewer) Review(ctx context.Context, namespace, verb, group, resource, subresource string) (bool, string, error) {
	name := capabilityNameForReview(verb, group, resource, subresource)
	return f.allowed[name], "", nil
}

func capabilityNameForReview(verb, group, resource, subresource string) string {
	switch {
	case verb == "get" && resource == "services":
		return CapabilityServicesGet
	case verb == "list" && resource == "services":
		return CapabilityServicesList
	case verb == "get" && resource == "endpoints":
		return CapabilityEndpointsGet
	case verb == "list" && resource == "endpoints":
		return CapabilityEndpointsList
	case verb == "get" && resource == "pods":
		return CapabilityPodsGet
	case verb == "list" && resource == "pods":
		return CapabilityPodsList
	case verb == "create" && resource == "pods" && subresource == "portforward":
		return CapabilityPodsPortForward
	case verb == "create" && group == "apps" && resource == "deployments":
		return CapabilityDeployments
	case verb == "create" && resource == "secrets":
		return CapabilitySecrets
	case verb == "create" && resource == "configmaps":
		return CapabilityConfigMaps
	default:
		return verb + "." + group + "." + resource + "." + subresource
	}
}

func TestProbeCapabilities(t *testing.T) {
	allowed := map[string]bool{
		CapabilityServicesGet:     true,
		CapabilityServicesList:    true,
		CapabilityEndpointsGet:    true,
		CapabilityPodsGet:         true,
		CapabilityPodsList:        true,
		CapabilityPodsPortForward: true,
	}
	caps := ProbeCapabilities(context.Background(), "ns-a", fakeReviewer{allowed: allowed})
	byName := map[string]Capability{}
	for _, cap := range caps {
		byName[cap.Name] = cap
		if cap.Namespace != "ns-a" {
			t.Fatalf("expected namespace ns-a, got %q", cap.Namespace)
		}
	}
	if !byName[CapabilityKubeconfig].Allowed {
		t.Fatal("kubeconfig capability should be allowed when environment loaded")
	}
	if !byName[CapabilityPodsPortForward].Allowed {
		t.Fatal("expected port-forward allowed")
	}
	if byName[CapabilityDeployments].Allowed {
		t.Fatal("deployment create should not be allowed by fake reviewer")
	}
}
