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
	"fmt"
	"os"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/hash"
	"github.com/RonsenbergVI/fraise/pkg/logger"
	"github.com/RonsenbergVI/fraise/pkg/server"
)

func PrintBanner() {
	fmt.Print(`
	██████ ▄▄▄▄   ▄▄▄  ▄▄  ▄▄▄▄ ▄▄▄▄▄
	██▄▄   ██▄█▄ ██▀██ ██ ███▄▄ ██▄▄
	██     ██ ██ ██▀██ ██ ▄▄██▀ ██▄▄▄
	`)
}

func main() {
	PrintBanner()

	c := config.New()
	cfgErr := c.Parse(os.Args[1:]) // load config file, then override via CLI flags

	logger.SetDefault(logger.NewLogger(c))

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
	var err error
	switch c.DB.Precision {
	case "float32":
		logger.Info("Using single precision", "precision", "float32")
		err = runServer[float32](c)
	case "float64":
		logger.Info("Using double precision", "precision", "float64")
		err = runServer[float64](c)
	default:
		logger.Warn("Unknown precision, falling back to float64", "precision", c.DB.Precision)
		err = runServer[float64](c)
	}

	if err != nil {
		logger.Error("Failed to start server", "error", err)
	}
}

// run builds a server at the requested floating-point precision and starts it.
// K is fixed to uint64 (the hasher's key type); only P varies with config.
func runServer[P float32 | float64](c *config.ConfigSet) error {
	srv := server.New[uint64, P](c, hash.NewHasher[uint64](c))
	return srv.Start()
}
