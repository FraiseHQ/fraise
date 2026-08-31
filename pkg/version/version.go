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

import "strings"

// Version is the release identity compiled into every binary, and the single
// place the version number lives in the tree.
//
// The trailing annotation is release-please's marker: when it opens a release
// PR it rewrites the literal on this line to the version being cut, in the same
// commit that updates CHANGELOG.md, and tags that commit on merge. The tag and
// this constant therefore cannot drift — do not edit it by hand.
//
// GoReleaser additionally overrides it via -ldflags -X on a release build. The
// linker only rewrites string variables that are uninitialized or set to a
// constant expression, so this must stay a plain literal — never computed.
var Version = "0.1.0-rc.4" // x-release-please-version

var (
	// Commit is the short git commit the binary was built from.
	Commit = "none"
	// Date is the commit/build date (RFC3339) of the release.
	Date = "unknown"
)

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
