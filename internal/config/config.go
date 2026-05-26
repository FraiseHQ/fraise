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

package config

import (
	"flag"
)

const DefaultConfigFile = "fraise.config.toml"

const (
	HTTPSPort             = "HTTPSPort"
	AllowUnanchoredRecall = "AllowUnanchoredRecall"
)

// Config
type ConfigSet struct {
	*flag.FlagSet `json:"-"`

	Server ServerConfig `toml:"server"`
	Log    LogConfig    `toml:"log"`
	Engine EngineConfig `toml:"engine"`
	DB     DBConfig     `toml:"db"`
}

type ServerConfig struct {
	HTTPSPort int `toml:"port"`
}

type LogConfig struct {
	// LOG LEVEL: DEBUG, INFO, WARN, ERROR (default = INFO)
	Level string `toml:"level"`

	// LOG FORMAT: console, json or text (default = console)
	Format string `toml:"format"`

	// Disable log timestamp
	DisableTimestamp bool `toml:"disable-timestamp"`
}

type EngineConfig struct {
	AllowUnanchoredRecall bool `toml:"allow-unanchored-recall"`
}

type DBConfig struct {
}

func Parse() {}
