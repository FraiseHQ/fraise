// MIT License

// Copyright (c) 2026 René-Jean Corneille

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package version_test

import (
	"regexp"
	"testing"

	"github.com/RonsenbergVI/fraise/pkg/version"
)

// semverPattern is MAJOR.MINOR.PATCH with an optional dot-numbered pre-release
// suffix — the only shapes the release process produces (see RELEASE.md).
var semverPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?$`)

// setVersion overrides version.Version for one test and restores it afterwards,
// so tests that exercise the version-derived helpers don't leak into each other.
func setVersion(t *testing.T, v string) {
	t.Helper()
	prev := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = prev })
}

// TestCompiledVersionIsWellFormed covers the dev-build path: with no -ldflags
// injection the compiled literal is reported verbatim, so a malformed value
// ships as the reported version. It pins the shape, not the number — the number
// changes every release, and release-please owns it.
func TestCompiledVersionIsWellFormed(t *testing.T) {
	if version.Version == "" {
		t.Fatal("Version is empty: dev builds report this literal directly")
	}
	if !semverPattern.MatchString(version.Version) {
		t.Errorf("Version = %q, want MAJOR.MINOR.PATCH with an optional -alpha.N/-beta.N/-rc.N suffix",
			version.Version)
	}
	if got := version.FullVersion(); got != version.Version {
		t.Errorf("FullVersion() = %q, want the compiled literal %q", got, version.Version)
	}
}

// TestFullVersionReflectsInjection simulates GoReleaser setting version.Version
// via -ldflags: FullVersion must report exactly what was injected.
func TestFullVersionReflectsInjection(t *testing.T) {
	setVersion(t, "1.4.0")
	if got := version.FullVersion(); got != "1.4.0" {
		t.Errorf("FullVersion() = %q, want %q", got, "1.4.0")
	}
}

func TestShortVersion(t *testing.T) {
	cases := []struct {
		name    string
		version string
		want    string
	}{
		{"plain semver", "1.2.3", "1.2"},
		{"pre-release suffix", "1.2.3-rc.1", "1.2"},
		{"v prefix", "v2.5.0", "2.5"},
		{"v prefix with pre-release", "v1.2.3-alpha.1", "1.2"},
		{"compiled default", "0.1.0-alpha.1", "0.1"},
		{"build metadata", "2.0.0-beta.2+build.5", "2.0"},
		{"multi-digit", "10.20.30", "10.20"},
		{"already short", "1.2", "1.2"},
		{"major only", "1", "1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setVersion(t, tc.version)
			if got := version.ShortVersion(); got != tc.want {
				t.Errorf("ShortVersion() with Version=%q = %q, want %q", tc.version, got, tc.want)
			}
		})
	}
}

// TestBuildIdentityDefaults documents the placeholder values used off a release,
// so a change to them is a deliberate, reviewed edit.
func TestBuildIdentityDefaults(t *testing.T) {
	if version.Commit == "" {
		t.Error("Commit should have a non-empty default (e.g. \"none\") for dev builds")
	}
	if version.Date == "" {
		t.Error("Date should have a non-empty default (e.g. \"unknown\") for dev builds")
	}
}
