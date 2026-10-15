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

package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FraiseHQ/fraise/internal/config"
)

// args builds the arguments main hands to Parse once the command is stripped.
// -config names a missing file, so the run ignores any fraise.config.toml in
// the working directory. The flags under test are the whole configuration.
func args(t *testing.T, flags ...string) []string {
	t.Helper()

	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")
	return append([]string{"-config", missing}, flags...)
}

// TestRunRefusesToStartOnAnInvalidValue pins the startup contract where it is
// decided. run turns a rejected setting into a non-zero exit. main only
// prints the error and calls os.Exit(1). config.Parse can only report the
// problem, and run decides that it is fatal.
//
// Nothing is listening when this returns. run gives up before it builds a
// database or binds a port.
func TestRunRefusesToStartOnAnInvalidValue(t *testing.T) {
	cases := []struct {
		flag, value, setting string
	}{
		{"-log-level", "verbose", "log.level"},
		{"-log-format", "console", "log.format"},
		{"-precision", "floast32", "db.precision"},
		{"-hashing-function", "murmur", "db.hashing-function.name"},
		{"-search-algorithm", "dfs", "db.search-algorithm.name"},
		{"-workers", "-1", "scheduler.workers"},
		{"-max-depth", "3", "db.max-depth"},
	}

	for _, tc := range cases {
		t.Run(tc.flag+"="+tc.value, func(t *testing.T) {
			c := config.New()
			cfgErr := c.Parse(args(t, tc.flag, tc.value))
			err := run("serve", context.Background(), c, cfgErr)

			if !errors.Is(err, config.ErrInvalidValue) {
				t.Fatalf("run() = %v, want an ErrInvalidValue so main exits non-zero", err)
			}
			if !strings.Contains(err.Error(), tc.setting) {
				t.Errorf("error %q does not name the setting %q", err, tc.setting)
			}
			if !strings.Contains(err.Error(), "accepted:") {
				t.Errorf("error %q does not list the accepted values", err)
			}
		})
	}
}

// TestRunAcceptsAnyCasingOfAValueThatIsAccepted is the other side of the same
// decision. An operator types `-log-level error`, and that must not stop the
// server. run cannot be called with a value it accepts, since it would serve
// until a signal. So the test checks the configuration run would get, through
// the same Parse call main makes.
func TestRunAcceptsAnyCasingOfAValueThatIsAccepted(t *testing.T) {
	cases := []struct{ flag, value string }{
		{"-log-level", "error"},
		{"-log-format", "Json"},
		{"-precision", "Float64"},
		{"-hashing-function", "T1ha"},
		{"-search-algorithm", "EXCESS"},
	}

	for _, tc := range cases {
		t.Run(tc.flag+"="+tc.value, func(t *testing.T) {
			c := config.New()
			err := c.Parse(args(t, tc.flag, tc.value))

			if errors.Is(err, config.ErrInvalidValue) {
				t.Fatalf("%s %s would stop startup: %v", tc.flag, tc.value, err)
			}
		})
	}
}

// TestRunSurvivesAMissingConfigFile pins the line between the two failures run
// tells apart. A missing config file is reported but survivable. The defaults
// are a valid configuration, and containers ship without a file. So it must
// not take the fatal branch, or the server could not start without a file.
func TestRunSurvivesAMissingConfigFile(t *testing.T) {
	c := config.New()
	err := c.Parse(args(t))

	if !errors.Is(err, config.ErrMissingFile) {
		t.Fatalf("Parse with no config file = %v, want ErrMissingFile reported", err)
	}
	if errors.Is(err, config.ErrInvalidValue) || errors.Is(err, config.ErrParsingFailed) {
		t.Fatalf("a missing config file took the fatal branch: %v", err)
	}
}

// TestRunRefusesToStartOnAnUnusableConfigFile pins where the survivable line
// sits. Only a missing file is survivable. A file that is malformed, or names
// a key no setting has, stops run before it builds anything. Starting on
// defaults would treat an operator's mistyped key as never written.
func TestRunRefusesToStartOnAnUnusableConfigFile(t *testing.T) {
	for _, contents := range []string{
		"[log\nlevel = \"DEBUG\"\n",
		"[engine]\nallow-unanchored-recall = true\n",
	} {
		t.Run(contents, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), config.DefaultConfigFile)
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatalf("writing config file: %v", err)
			}

			c := config.New()
			cfgErr := c.Parse([]string{"-config", path})
			err := run("serve", context.Background(), c, cfgErr)

			if !errors.Is(err, config.ErrParsingFailed) {
				t.Fatalf("run() = %v, want ErrParsingFailed so main exits non-zero", err)
			}
		})
	}
}

// TestRunRejectsAnUnknownCommand pins the dispatch contract. A mistyped
// command is an error that names the accepted ones. It never falls through
// into serving. If `fraise sevre -config x` started the server, the running
// process would hide the typo.
func TestRunRejectsAnUnknownCommand(t *testing.T) {
	c := config.New()
	cfgErr := c.Parse(args(t))
	err := run("sevre", context.Background(), c, cfgErr)

	if err == nil || !strings.Contains(err.Error(), `"sevre"`) {
		t.Fatalf("run(sevre) = %v, want an error naming the unknown command", err)
	}
	if !strings.Contains(err.Error(), "serve, mcp or version") {
		t.Errorf("error %q does not list the accepted commands", err)
	}
}

// TestRunTreatsHelpAsSuccess pins the -h contract. The flag package answers
// help by printing usage and returning flag.ErrHelp. Help is a request
// served, not a failure, so run returns nil without dispatching the command.
//
// The command names something run rejects on purpose. The assertion can then
// tell "help short-circuited" from "help fell through into the dispatch". It
// does so without starting a server, which "serve" here would do.
func TestRunTreatsHelpAsSuccess(t *testing.T) {
	for _, name := range []string{"-h", "-help"} {
		t.Run(name, func(t *testing.T) {
			c := config.New()
			c.SetOutput(io.Discard) // the usage text is not this test's business
			cfgErr := c.Parse(args(t, name))

			if !errors.Is(cfgErr, flag.ErrHelp) {
				t.Fatalf("Parse(%s) = %v, want flag.ErrHelp", name, cfgErr)
			}
			if err := run("no-such-command", context.Background(), c, cfgErr); err != nil {
				t.Errorf("run() = %v, want nil: help is answered, not failed", err)
			}
		})
	}
}

// TestRunRejectsAnUnknownFlag pins that a mistyped flag is fatal. The flag set
// uses flag.ContinueOnError, so an unknown flag arrives as an ordinary error.
// run warns about ordinary errors and starts anyway. Parse classifies this
// one as ErrInvalidFlag, so it stays fatal here. Otherwise the server would
// start on a default the operator never asked for.
func TestRunRejectsAnUnknownFlag(t *testing.T) {
	c := config.New()
	c.SetOutput(io.Discard)
	cfgErr := c.Parse(args(t, "-bogus"))

	if !errors.Is(cfgErr, config.ErrInvalidFlag) {
		t.Fatalf("Parse(-bogus) = %v, want it to wrap ErrInvalidFlag", cfgErr)
	}
	if !strings.Contains(cfgErr.Error(), "bogus") {
		t.Errorf("error %q does not name the offending flag", cfgErr)
	}
	if err := run("serve", context.Background(), c, cfgErr); !errors.Is(err, config.ErrInvalidFlag) {
		t.Errorf("run() = %v, want the flag error so main exits non-zero", err)
	}
}
