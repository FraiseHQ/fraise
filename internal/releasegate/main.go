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

// Releasegate is the pre-release version gate, run by the publish job before
// anything is built: `go run ./internal/releasegate <tag>`. It compiles
// pkg/version without any ldflags injection and compares the resulting
// Major.Minor.Patch fallback against the tag being cut. The two must match
// exactly, because builds of the tagged source that skip GoReleaser's ldflags
// (go install m@tag, the docker image, make build) report the compiled
// fallback — a mismatch means those builds would ship claiming a stale
// version. Bumping the fallback belongs in the release PR (see RELEASE.md).
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/RonsenbergVI/fraise/pkg/version"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: releasegate <tag>")
		os.Exit(2)
	}
	tag := os.Args[1]

	compiled := fmt.Sprintf("%d.%d.%s", version.Major, version.Minor, version.Patch)
	if want := strings.TrimPrefix(tag, "v"); compiled != want {
		fmt.Fprintf(os.Stderr,
			"releasegate: pkg/version compiled fallback is %q but tag %s wants %q\n"+
				"bump Major/Minor/Patch in pkg/version/version.go (in the release PR) before tagging\n",
			compiled, tag, want)
		os.Exit(1)
	}
	fmt.Printf("releasegate: pkg/version fallback %s matches tag %s\n", compiled, tag)
}
