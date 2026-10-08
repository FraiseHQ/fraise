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

// Package config resolves the server configuration from built-in defaults, a
// TOML file and command-line flags, and rejects any value the server cannot
// honour before it starts.
//
// [New] returns a [ConfigSet] holding the defaults with a flag bound to every
// setting. [ConfigSet.Parse] decodes the file (fraise.config.toml unless
// -config names another) over the defaults, applies the flags over the file,
// gives every setting still at its zero value its default, and validates:
// each fixed-vocabulary setting is matched case-insensitively and rewritten to
// its canonical spelling, so consumers compare with ==, and bounded numbers
// are range-checked. A missing file is [ErrMissingFile], the one survivable
// failure, since the defaults and flags are a complete configuration. An
// unknown key, a malformed file, a bad flag or an invalid value stops
// startup: a server running on defaults the operator believes they overrode
// is worse than one that does not start.
package config
