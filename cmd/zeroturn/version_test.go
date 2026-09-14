package main

import (
	"strings"
	"testing"
)

// A build made with go install carries no version from the release
// script, and reporting a development version to somebody who installed
// a published one is a lie about what they are running.
func TestVersionPrefersWhatTheBuildSet(t *testing.T) {
	saved := Version
	defer func() { Version = saved }()

	Version = "1.2.3"
	if got := version(); got != "1.2.3" {
		t.Fatalf("version() = %q, want the value the build set", got)
	}
}

// From a checkout there is no module version to fall back to, so the
// default stands rather than something misleading.
func TestVersionFallsBackToTheDefault(t *testing.T) {
	if got := version(); got == "" {
		t.Fatal("version() is empty")
	}
	if strings.Contains(version(), "(devel)") {
		t.Fatalf("version() reports the Go placeholder: %q", version())
	}
}
