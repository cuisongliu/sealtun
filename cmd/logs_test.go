package cmd

import (
	"testing"
	"time"
)

func TestValidateLogOptionsRejectsNegativeValues(t *testing.T) {
	t.Parallel()

	if err := validateLogOptions(-1, 0); err == nil {
		t.Fatal("expected negative --tail to be rejected")
	}
	if err := validateLogOptions(100, -time.Second); err == nil {
		t.Fatal("expected negative --since to be rejected")
	}
}

func TestValidateLogOptionsAllowsZeroValues(t *testing.T) {
	t.Parallel()

	if err := validateLogOptions(0, 0); err != nil {
		t.Fatalf("expected zero values to be allowed: %v", err)
	}
}
