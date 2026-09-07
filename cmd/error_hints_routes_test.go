package cmd

import "testing"

// Route validation errors are self-explanatory (they already name the fix),
// so the hint system must stay silent for them. This guards against a broad
// hint pattern (e.g. anything containing "route") misfiring on the newer
// multi-service route messages.
func TestRouteValidationErrorsGetNoHint(t *testing.T) {
	t.Parallel()

	messages := []string{
		`invalid route "/api"; expected the form /path=port, for example /api=8080`,
		`invalid route "/api=abc": port must be a number`,
		`route path "api" must start with /`,
		`route path "/api?x=1" must be a plain path prefix without query, fragment, or whitespace`,
		`route path "/api%2Fusers" must not contain % escapes or dot segments; request paths arrive decoded and cleaned, so such a prefix can never match`,
		`duplicate route for path prefix "/api"`,
		`route "/api" has invalid port 0; must be between 1 and 65535`,
		`--route is only supported for https tunnels`,
		`--route cannot be combined with --target; a target upstream is a single service`,
		`tunnel app: routes are only supported for https tunnels`,
		`tunnel app: routes cannot be combined with target; a target upstream is a single service`,
		`tunnel app routes: duplicate route for path prefix "/api"`,
		`port "03000" has a leading zero and is parsed as octal by YAML; write the port without the leading zero (e.g. 3000)`,
	}
	for _, msg := range messages {
		if hint := actionableErrorHintText(msg); hint != "" {
			t.Fatalf("route validation error should produce no hint, got %q for %q", hint, msg)
		}
	}
}
