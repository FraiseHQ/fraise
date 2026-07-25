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
	_ = c.Parse(os.Args[1:]) // override config via CLI flags

	logger.SetDefault(logger.NewLogger(c))

	logger.Info("Starting server...")
	logger.Debug("Config", c)

	// MurmurHash produces uint32 keys, so the server is instantiated with
	// K = uint32; float64 is used for embedding/score precision.
	srv := server.New[uint32, float64](c, hash.MurmurHash{})

	if err := srv.Start(); err != nil {
		logger.Error("Failed to start server", "error", err)
	}
}
