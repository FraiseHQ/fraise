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

package version

import (
	"fmt"
	"strings"
)

var (
	// Major is the current major version of main branch.
	Major = 0
	// Minor is the current minor version of main branch.
	Minor = 1
	// Patch is the current patched version of the main branch. It is a string
	// so it can also carry a pre-release suffix, e.g. "0-rc.1" -> 0.1.0-rc.1.
	Patch = "0-alpha.1"
)

// Version, Commit and Date are the build-time release identity. On a release
// they are injected by GoReleaser via -ldflags -X (see .goreleaser.yaml); the
// linker can only set string variables that are uninitialized or set to a
// constant, so these are plain strings — never computed with fmt.Sprintf.
//
// In a dev build Version is empty and init() falls back to the compiled
// Major.Minor.Patch above, so the numbers in this file remain the source of
// truth off a release.
var (
	// Version is the full semantic version, e.g. "1.2.3" or "1.2.3-rc.1".
	Version string
	// Commit is the short git commit the binary was built from.
	Commit = "none"
	// Date is the commit/build date (RFC3339) of the release.
	Date = "unknown"
)

func init() {
	if Version == "" {
		Version = fmt.Sprintf("%d.%d.%s", Major, Minor, Patch)
	}
}

// FullVersion returns the full version string, e.g. "1.2.3".
func FullVersion() string { return Version }

// ShortVersion returns just the "major.minor" prefix, e.g. "1.2", derived from
// the resolved Version so it stays correct whether that came from GoReleaser or
// the compiled defaults.
func ShortVersion() string {
	v := strings.TrimPrefix(Version, "v")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i] // drop any pre-release suffix
	}
	parts := strings.SplitN(v, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return v
}
