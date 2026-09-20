package auth

import (
	"fmt"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// RewriteKubeconfigNamespace returns a copy of the kubeconfig with every
// context's namespace set to the given workspace namespace. The regional
// token endpoint only issues kubeconfigs for the login's default workspace,
// so switching workspaces means rewriting the namespace locally; the token's
// RBAC coverage in the target workspace is verified by callers before use.
func RewriteKubeconfigNamespace(kubeconfig, namespace string) (string, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	config, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		return "", fmt.Errorf("parse kubeconfig: %w", err)
	}
	if len(config.Contexts) == 0 {
		return "", fmt.Errorf("kubeconfig has no contexts")
	}
	for name, ctx := range config.Contexts {
		ctx.Namespace = namespace
		config.Contexts[name] = ctx
	}
	out, err := clientcmd.Write(*config)
	if err != nil {
		return "", fmt.Errorf("encode kubeconfig: %w", err)
	}
	return string(out), nil
}

// KubeconfigNamespace returns the namespace of the current context.
func KubeconfigNamespace(kubeconfig string) (string, error) {
	config, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		return "", fmt.Errorf("parse kubeconfig: %w", err)
	}
	return currentContextNamespace(config), nil
}

func currentContextNamespace(config *clientcmdapi.Config) string {
	if config.CurrentContext == "" {
		return "default"
	}
	ctx, ok := config.Contexts[config.CurrentContext]
	if !ok || ctx.Namespace == "" {
		return "default"
	}
	return ctx.Namespace
}
