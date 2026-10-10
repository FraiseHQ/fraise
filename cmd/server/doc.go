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

// Command fraise runs the Fraise temporal memory graph database, the MCP
// bridge to it, or reports its version.
//
// Usage:
//
//	fraise [serve|mcp|version] [flags]
//
// The first argument selects the subcommand; when it is absent or is a flag,
// the subcommand is serve, so 'fraise -config fraise.config.toml' starts the
// server. The subcommands are:
//
//   - serve starts the HTTP daemon at the configured precision and runs until
//     SIGINT or SIGTERM, then drains in-flight requests before exiting.
//   - mcp serves the Model Context Protocol over stdio, forwarding the recall
//     and remember tools to the daemon at -addr (by default the one the same
//     config describes, on 127.0.0.1 at -port).
//   - version prints the release version.
//
// Every setting has a flag; -config names the TOML file read first (default
// fraise.config.toml), and flags override it. A missing config file falls
// back to the built-in defaults; an invalid value, an unknown flag or an
// unusable file stops startup. The process exits non-zero on any failure, so a
// supervisor's restart policy can see it.
package main
