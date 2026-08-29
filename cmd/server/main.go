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
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/pkg/logger"
	"github.com/FraiseHQ/fraise/pkg/server"
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
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		if err := runMCP(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "fraise mcp:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		logger.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}

// run wires up config, signals and the server, and blocks until the server
// stops. It returns an error instead of exiting so its deferred signal stop
// runs and main can translate failure into the process exit code.
func run() error {
	PrintBanner()

	// A context cancelled on SIGINT/SIGTERM drives graceful shutdown: an
	// operator's `docker stop`/Ctrl-C lets in-flight writes finish instead of
	// being dropped. stop restores default signal handling on return.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := config.New()
	cfgErr := c.Parse(os.Args[1:]) // load config file, then override via CLI flags

	// The logger is installed before the config error is acted on, so that the
	// error has somewhere to go — main reports it through this logger.
	logger.SetDefault(logger.NewLogger(c))

	// A setting naming something the server cannot honour stops startup, before
	// anything is announced: it was asked for explicitly, and running with a
	// different value instead is a substitution an operator can only detect by
	// noticing the behaviour they asked for is missing. The error lists what the
	// setting accepts.
	if errors.Is(cfgErr, config.ErrInvalidValue) {
		return cfgErr
	}

	logger.Info("Starting server...")
	if cfgErr != nil {
		// Parse falls back to built-in defaults on a missing/invalid file; log
		// so a silently-defaulted config is visible rather than a surprise.
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
	return srv.Start(ctx)
}
