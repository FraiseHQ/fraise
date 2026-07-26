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

package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/config"
)

func TestConfigSet_FromFile(t *testing.T) {
	const contents = `
[scheduler]
workers = 4
buffer-size = 128

[server]
port = 8080

[log]
level = "DEBUG"
format = "json"
disable-timestamp = true

[engine]
allow-unanchored-recall = true
half-life = "168h"
cache-capacity = 1024

[db]
default-top = 10
default-depth = 3
seed-size = 64
hop-attenuation = 0.5

[db.hashing-function]
name = "xxhash"
`

	dir := t.TempDir()
	path := filepath.Join(dir, config.DefaultConfigFile)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	c := config.New()
	if _, err := c.FromFile(path); err != nil {
		t.Fatalf("FromFile returned error: %v", err)
	}

	if c.Scheduler.Workers != 4 {
		t.Errorf("Scheduler.Workers: got %d, want 4", c.Scheduler.Workers)
	}
	if c.Scheduler.BufferSize != 128 {
		t.Errorf("Scheduler.BufferSize: got %d, want 128", c.Scheduler.BufferSize)
	}
	if c.Server.Port != 8080 {
		t.Errorf("Server.Port: got %d, want 8080", c.Server.Port)
	}
	if c.Log.Level != "DEBUG" {
		t.Errorf("Log.Level: got %q, want %q", c.Log.Level, "DEBUG")
	}
	if c.Log.Format != "json" {
		t.Errorf("Log.Format: got %q, want %q", c.Log.Format, "json")
	}
	if !c.Log.DisableTimestamp {
		t.Errorf("Log.DisableTimestamp: got false, want true")
	}
	if !c.Engine.AllowUnanchoredRecall {
		t.Errorf("Engine.AllowUnanchoredRecall: got false, want true")
	}
	if c.Engine.Halflife != 168*time.Hour {
		t.Errorf("Engine.Halflife: got %v, want %v", c.Engine.Halflife, 168*time.Hour)
	}
	if c.DB.SeedSize != 64 {
		t.Errorf("DB.SeedSize: got %d, want 64", c.DB.SeedSize)
	}
	if c.DB.HopAttenuation != 0.5 {
		t.Errorf("DB.HopAttenuation: got %v, want 0.5", c.DB.HopAttenuation)
	}
	if c.Engine.CacheCapacity != 1024 {
		t.Errorf("Engine.CacheCapacity: got %d, want 1024", c.Engine.CacheCapacity)
	}
	if c.DB.DefaultTop != 10 {
		t.Errorf("DB.DefaultTop: got %d, want 10", c.DB.DefaultTop)
	}
	if c.DB.DefaultDepth != 3 {
		t.Errorf("DB.DefaultDepth: got %d, want 3", c.DB.DefaultDepth)
	}
	if c.DB.HashingFunction.Name != "xxhash" {
		t.Errorf("DB.HashingFunction.Name: got %q, want %q", c.DB.HashingFunction.Name, "xxhash")
	}
}

func TestConfigSet_FromFile_Missing(t *testing.T) {
	c := config.New()
	if _, err := c.FromFile(filepath.Join(t.TempDir(), "does-not-exist.toml")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
