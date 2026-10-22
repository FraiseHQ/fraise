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
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/pkg/logger"
	"github.com/FraiseHQ/fraise/pkg/mcp"
	"github.com/FraiseHQ/fraise/pkg/server"
	"github.com/FraiseHQ/fraise/pkg/version"
)

func PrintBanner() {
	fmt.Print(`

	██████ ▄▄▄▄   ▄▄▄  ▄▄  ▄▄▄▄ ▄▄▄▄▄
	██▄▄   ██▄█▄ ██▀██ ██ ███▄▄ ██▄▄
	██     ██ ██ ██▀██ ██ ▄▄██▀ ██▄▄▄

	`)
}

// main is the only place that exits. run owns every defer, so os.Exit here
// cannot skip one (gocritic exitAfterDefer). A non-zero exit lets a
// supervisor, such as a docker or k8s on-failure restart policy, see the
// startup failure. Exiting 0 would mark a dead server as a clean shutdown.
func main() {

	args := os.Args[1:]

	// The first argument selects the command when it is not a flag. Otherwise
	// the command is serve, so a bare 'fraise -config x' starts the server.
	// The docker command, the systemd unit and brew services all run it that
	// way. The command is stripped before flag parsing, because Parse rejects
	// any non-flag argument.
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}

	c := config.New()
	cfgErr := c.Parse(args) // load config file, then override via CLI flags

	if err := run(cmd, context.Background(), c, cfgErr); err != nil {
		// Write to stderr, not the logger. On the mcp path stdout belongs to
		// the protocol, and no stdout logger is ever installed.
		fmt.Fprintln(os.Stderr, "fraise:", err)
		os.Exit(1)
	}
}

// run dispatches the command and blocks until it finishes. It returns an
// error instead of exiting, so its deferred signal stop runs. main turns the
// failure into the process exit code.
func run(cmd string, ctx context.Context, c *config.ConfigSet, cfgErr error) error {

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	// SIGINT or SIGTERM cancels ctx, which drives a graceful shutdown. An
	// operator's 'docker stop' or Ctrl-C lets in-flight writes finish instead
	// of dropping them. This lives in run, not main, so os.Exit cannot skip
	// the deferred stop that restores default signal handling.
	defer stop()

	// -h and --help are requests answered, not failures. The flag package has
	// already written the usage text, and nothing is left to run. Help is
	// handled here, not by flag.ExitOnError, so every exit decision stays in
	// one place the tests can reach.
	if errors.Is(cfgErr, flag.ErrHelp) {
		return nil
	}

	// An invalid value, an unrecognised flag and an unusable config file stop
	// startup on every path, before anything is announced. A file is unusable
	// when it is malformed or names a key no setting has. These are checked
	// here because everything below only warns. A warning would start the
	// server on a default the operator never asked for. The one survivable
	// failure is a missing file.
	if errors.Is(cfgErr, config.ErrInvalidValue) || errors.Is(cfgErr, config.ErrInvalidFlag) || errors.Is(cfgErr, config.ErrParsingFailed) {
		return cfgErr
	}

	switch cmd {
	case "mcp":
		// Stdout carries the MCP protocol from here on, so there is no logger
		// and no banner. Fatal config errors were returned above, and main
		// prints them on stderr.
		return mcp.New(c).Start(ctx)

	case "serve":
		// Install the logger before acting on the config error, so the error
		// has somewhere to go.
		logger.SetDefault(logger.NewLogger(c))
		logger.Info("Starting server...")
		if cfgErr != nil {
			// Only a missing file survives to here. Parse fell back to the
			// built-in defaults, and the warning makes that visible.
			logger.Warn("Config file not found, using defaults", "error", cfgErr)
		}
		logger.Debug("Config loaded", "config", c)

		// P, the embedding and score precision, is a compile-time type
		// parameter. The config value selects which instantiation to build
		// and run. Both are compiled in, and the whole stack below is generic
		// over P. Parse has already defaulted an absent precision and rejected
		// any other value, so float64 is the only value that is not float32.
		if c.DB.Precision == config.PrecisionFloat64 {
			logger.Info("Using double precision", "precision", config.PrecisionFloat64)
			return runServer[float64](ctx, c)
		}
		logger.Info("Using single precision", "precision", config.PrecisionFloat32)
		return runServer[float32](ctx, c)

	case "version":
		fmt.Println(version.Version)
		return nil

	default:
		return fmt.Errorf("unknown command %q (expected serve, mcp or version)", cmd)
	}
}

// runServer builds a server at the requested floating-point precision and
// starts it. K is fixed to uint64, the hasher's key type. Only P varies with
// config. Cancelling ctx starts a graceful shutdown, and Start returns once
// the stack has drained.
func runServer[P float32 | float64](ctx context.Context, c *config.ConfigSet) error {
	srv, err := server.New[uint64, P](c, hash.NewHasher[uint64](c))
	if err != nil {
		return err
	}
	PrintBanner()
	return srv.Start(ctx)
}
