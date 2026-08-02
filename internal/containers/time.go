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

package containers

import (
	"fmt"
	"strconv"
	"time"
)

type TimeValue interface {
	Resolve(now time.Time) time.Time
}

type RelativeTime struct{ Dur time.Duration }
type AbsoluteTime struct{ T time.Time }

func (r RelativeTime) String() string {
	return r.Dur.String()
}

func (a AbsoluteTime) String() string {
	return a.T.Format(time.RFC822)
}

func (r RelativeTime) Resolve(now time.Time) time.Time { return now.Add(-r.Dur) }
func (a AbsoluteTime) Resolve(_ time.Time) time.Time   { return a.T }

type TimeFilter struct {
	Dur   time.Duration
	Abs   time.Time
	IsAbs bool
}

func (tf TimeFilter) Resolve(now time.Time) time.Time {
	if tf.IsAbs {
		return tf.Abs
	}
	return now.Add(-tf.Dur)
}

// ParseTimeValue converts a string into a TimeValue: a RelativeTime
// ("7d", "30m", "1w") or an AbsoluteTime ("2026-01-15" or RFC3339).
func ParseTimeValue(s string) (TimeValue, error) {
	if s == "" {
		return nil, fmt.Errorf("%w: empty string", ErrInvalidTime)
	}

	// Relative: <int><unit>. A date never ends in a unit letter.
	if mult, ok := unitDuration(s[len(s)-1]); ok {
		if n, err := strconv.Atoi(s[:len(s)-1]); err == nil && n >= 0 {
			return RelativeTime{Dur: time.Duration(n) * mult}, nil
		}
	}

	// Absolute: ISO date or RFC3339 datetime.
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return AbsoluteTime{T: t}, nil
		}
	}

	return nil, fmt.Errorf("%w: %q (want e.g. 7d or 2026-01-15)", ErrInvalidTime, s)
}

func unitDuration(b byte) (time.Duration, bool) {
	switch b {
	case 's':
		return time.Second, true
	case 'm':
		return time.Minute, true
	case 'h':
		return time.Hour, true
	case 'd':
		return 24 * time.Hour, true
	case 'w':
		return 7 * 24 * time.Hour, true
	}
	return 0, false
}
