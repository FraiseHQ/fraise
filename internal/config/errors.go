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

import "errors"

var (
	// ErrMissingFile is returned when the config file does not exist. It is
	// the one config failure that is survivable: the built-in defaults plus
	// the flags are a complete configuration, and containers routinely ship
	// without a file.
	ErrMissingFile = errors.New("config: config file not found")
	// ErrParsingFailed is returned when the config file exists but cannot be
	// used: it is not valid TOML, a value has the wrong type, or a key maps to
	// no known setting. It stops startup like an invalid value does — a file
	// the server half-read would run with defaults the operator believes they
	// overrode, and a mistyped key is exactly the edit they meant to make.
	ErrParsingFailed = errors.New("config: error while parsing config file")
	// ErrInvalidFlag is returned when the command line carries an unexpected
	// positional argument (i.e. an unknown/invalid flag).
	ErrInvalidFlag = errors.New("config: invalid flag")
	// ErrInvalidValue is returned when a setting names something outside the
	// values it accepts. It is deliberately distinct from ErrMissingFile: a
	// missing config file is survivable (the defaults are a valid
	// configuration), but a value the server cannot honour is not, so the
	// startup path stops on this one instead of warning and carrying on.
	ErrInvalidValue = errors.New("config: invalid value")
)
