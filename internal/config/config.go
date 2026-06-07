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
	"time"

	"github.com/BurntSushi/toml"
)

// Config
type ConfigSet struct {
	*flag.FlagSet `json:"-"`

	Scheduler SchedulerConfig `toml:"scheduler"`
	Server    ServerConfig    `toml:"server"`
	Log       LogConfig       `toml:"log"`
	Engine    EngineConfig    `toml:"engine"`
	DB        DBConfig        `toml:"db"`
}

type SchedulerConfig struct {
	// Number of workers scheduler has to execute read and writes
	Workers int `toml:"workers"`

	// Buffer size is the scheduler channel buffer size.
	BufferSize int `toml:"buffer-size"`
}

type ServerConfig struct {
	Port int `toml:"port"`
}

type LogConfig struct {
	// LOG LEVEL: DEBUG, INFO, WARN, ERROR (default = INFO)
	Level string `toml:"level"`

	// LOG FORMAT: text or json (default = text)
	// Note: all logs are printed in console. File logging not supported (yet)
	Format string `toml:"format"`

	// Disable log timestamp
	DisableTimestamp bool `toml:"disable-timestamp"`
}

type EngineConfig struct {
	// Allow unanchored recalls: queries of type "recall since:7d"
	AllowUnanchoredRecall bool `toml:"allow-unanchored-recall"`

	// Half life for time decay (used to score facts)
	Halflife time.Duration `toml:"half-life"`

	// How many seeds to pull from each source (keywords and vector)
	SeedSize int `toml:"seed-size"`

	// Score attenuation for graph walk
	HopAttenuation float64 `toml:"hop-attenuation"`

	// Query cache size
	CacheCapacity int `toml:"cache-capacity"`
}

type DBConfig struct {
	DefaultTop int `toml:"default-top"`

	DefaultDepth int `toml:"default-depth"`
}

// Instanciates new configset
func New() *ConfigSet {
	config := &ConfigSet{}

	config.FlagSet = flag.NewFlagSet("flags", flag.PanicOnError)

	flagSet := config.FlagSet

	flagSet.IntVar(&config.Server.Port, "port", defaultPort, "Server port")

	return config
}

// Clones config set
func (c *ConfigSet) Clone() *ConfigSet {
	config := &ConfigSet{}
	*config = *c
	return config
}

// Returns config as string (useful for debugging)
func (c *ConfigSet) String() string {
	var result string

	return result
}

// Validates config
func (c *ConfigSet) Validate() error {
	return nil
}

// / Parses flag definition from argument list.
func (c *ConfigSet) Parse(arguments []string) error {
	return nil
}

func (c *ConfigSet) FromFile(path string) (*toml.MetaData, error) {
	return nil, nil
}
