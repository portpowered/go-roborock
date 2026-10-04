package main

import "testing"

func TestReleaseAllowsBreak(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		base        string
		release     string
		wantAllowed bool
	}{
		{name: "v0 minor permits break", base: "v0.1.9", release: "v0.2.0", wantAllowed: true},
		{name: "v0 patch rejects break", base: "v0.1.9", release: "v0.1.10", wantAllowed: false},
		{name: "stable minor rejects break", base: "v1.2.3", release: "v1.3.0", wantAllowed: false},
		{name: "major permits break", base: "v1.2.3", release: "v2.0.0", wantAllowed: true},
		{name: "v0 major permits break", base: "v0.9.9", release: "v1.0.0", wantAllowed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			allowed, _, err := releaseAllowsBreak(test.base, test.release)
			if err != nil {
				t.Fatalf("releaseAllowsBreak() error = %v", err)
			}

			if allowed != test.wantAllowed {
				t.Errorf("releaseAllowsBreak() = %t, want %t", allowed, test.wantAllowed)
			}
		})
	}
}

func TestParsePackages(t *testing.T) {
	t.Parallel()

	got, err := parsePackages("., httpclient, resources")
	if err != nil {
		t.Fatalf("parsePackages() error = %v", err)
	}

	if len(got) != 3 || got[0] != "." || got[1] != "httpclient" || got[2] != "resources" {
		t.Errorf("parsePackages() = %#v, want root and two packages", got)
	}
}

func TestParsePackagesRejectsDuplicateOrEscapingPaths(t *testing.T) {
	t.Parallel()

	for _, value := range []string{".,.", "../private", "httpclient,../private"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			_, err := parsePackages(value)
			if err == nil {
				t.Errorf("parsePackages(%q) error = nil, want error", value)
			}
		})
	}
}
