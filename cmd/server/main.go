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

// main is the only place that exits: run owns every defer, so os.Exit here
// cannot skip one (gocritic exitAfterDefer). A non-zero exit lets a
// supervisor (docker/k8s on-failure restart policy) see the startup failure;
// exiting 0 would mark a dead server as a clean shutdown.
func main() {

	args := os.Args[1:]

	// The first argument selects the command when it is not a flag; otherwise
	// the command defaults to the server, so every invocation that predates
	// subcommands — the docker CMD, the systemd unit, brew services, a bare
	// `fraise -config x` — keeps meaning what it always meant. The command is
	// stripped before flag parsing: Parse rejects any non-flag argument.
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}

	c := config.New()
	cfgErr := c.Parse(args) // load config file, then override via CLI flags

	if err := run(cmd, context.Background(), c, cfgErr); err != nil {
		// Stderr, not the logger: on the mcp path stdout belongs to the
		// protocol and no stdout logger is ever installed. A non-zero exit
		// lets a supervisor see the failure instead of a clean shutdown.
		fmt.Fprintln(os.Stderr, "fraise:", err)
		os.Exit(1)
	}
}

// run dispatches the command and blocks until it finishes. It returns an
// error instead of exiting so its deferred signal stop runs and main can
// translate failure into the process exit code.
func run(cmd string, ctx context.Context, c *config.ConfigSet, cfgErr error) error {

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	// A context cancelled on SIGINT/SIGTERM drives graceful shutdown: an
	// operator's `docker stop`/Ctrl-C lets in-flight writes finish instead of
	// being dropped — and it lives here, not in main, so os.Exit can never
	// skip the deferred restore. stop restores default signal handling.
	defer stop()

	// -h/--help is a request answered, not a failure: the flag package has
	// already written the usage text and there is nothing left to run. It is
	// handled here rather than by flag.ExitOnError so that every exit
	// decision stays in one place the tests can reach.
	if errors.Is(cfgErr, flag.ErrHelp) {
		return nil
	}

	// A setting naming something the process cannot honour stops startup on
	// every path, before anything is announced: it was asked for explicitly,
	// and running with a different value instead is a substitution an
	// operator can only detect by noticing the behaviour they asked for is
	// missing. The error lists what the setting accepts.
	//
	// An unrecognised flag is fatal for the same reason. It has to be named
	// explicitly because everything below merely warns: a mistyped flag that
	// warned would start the server with a default the operator never asked
	// for, silently.
	if errors.Is(cfgErr, config.ErrInvalidValue) || errors.Is(cfgErr, config.ErrInvalidFlag) {
		return cfgErr
	}

	switch cmd {
	case "mcp":
		// Stdout carries the MCP protocol from here on: no logger, no
		// banner. Config trouble was already handled above, on stderr.
		return mcp.New(c).Start(ctx)

	case "serve":
		// The logger is installed before the config error is acted on, so
		// that the error has somewhere to go.
		logger.SetDefault(logger.NewLogger(c))
		logger.Info("Starting server...")
		if cfgErr != nil {
			// Parse falls back to built-in defaults on a missing/invalid
			// file; log so a silently-defaulted config is visible rather
			// than a surprise.
			logger.Warn("Config not fully loaded, using defaults", "error", cfgErr)
		}
		logger.Debug("Config loaded", "config", c)

		// P (embedding/score precision) is a compile-time type parameter, so the
		// config value selects which instantiation to build and run here. Both are
		// compiled in; the whole stack below is generic over P.
		switch c.DB.Precision {
		case config.PrecisionFloat32:
			logger.Info("Using single precision", "precision", config.PrecisionFloat32)
			return runServer[float32](ctx, c)
		case config.PrecisionFloat64:
			logger.Info("Using double precision", "precision", config.PrecisionFloat64)
			return runServer[float64](ctx, c)
		default:
			// Startup rejects any other value, so this is an unset config, and the
			// documented default is what it means. It used to fall back to float64
			// — not even the default — so a typo silently changed the precision of
			// every score in the store.
			logger.Warn("Precision unset, using the default", "precision", config.DefaultPrecision)
			return runServer[float32](ctx, c)
		}

	case "version":
		fmt.Println(version.Version)
		return nil

	default:
		return fmt.Errorf("unknown command %q (expected serve, mcp or version)", cmd)
	}
}

// run builds a server at the requested floating-point precision and starts it.
// K is fixed to uint64 (the hasher's key type); only P varies with config. The
// context drives graceful shutdown: Start returns once it is cancelled and the
// stack has drained.
func runServer[P float32 | float64](ctx context.Context, c *config.ConfigSet) error {
	srv, err := server.New[uint64, P](c, hash.NewHasher[uint64](c))
	if err != nil {
		return err
	}
	PrintBanner()
	return srv.Start(ctx)
}
