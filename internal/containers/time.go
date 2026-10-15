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
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/FraiseHQ/fraise/internal/hash"
)

// TimeValue is a time bound that resolves itself against now. It hashes itself
// with the same Hasher[K, string] as its enclosing query, the way Recall and
// Remember do.
type TimeValue[K comparable] interface {
	Resolve(now time.Time) time.Time
	// Hash keys the bound through h and renders the key for folding into an
	// enclosing query's hash material. The material is lossless: a duration as
	// Duration.String renders it, an instant in RFC3339Nano. Each type adds a
	// distinct prefix, so different bounds never share material. If two did, a
	// recall would reuse the plan cached for another time window. String()
	// cannot serve as material, because AbsoluteTime's RFC822 form drops
	// seconds.
	Hash(h hash.Hasher[K, string]) string
}

// RelativeTime is a [TimeValue] a fixed span before now, as in since:7d. It
// moves with the clock, so the same query resolves to a later instant each
// time it runs.
type RelativeTime[K comparable] struct{ Dur time.Duration }

// AbsoluteTime is a [TimeValue] at a fixed instant, as in since:2026-01-15. It
// resolves to the same instant whenever the query runs.
type AbsoluteTime[K comparable] struct{ T time.Time }

func (r RelativeTime[K]) String() string {
	return r.Dur.String()
}

func (a AbsoluteTime[K]) String() string {
	return a.T.Format(time.RFC822)
}

// Resolve implements [TimeValue]: the instant Dur before now.
func (r RelativeTime[K]) Resolve(now time.Time) time.Time { return now.Add(-r.Dur) }

// Resolve implements [TimeValue]: T, whatever now is.
func (a AbsoluteTime[K]) Resolve(_ time.Time) time.Time { return a.T }

// Hash implements [TimeValue]. The material is "r" and the duration. No other
// bound's material starts with "r".
func (r RelativeTime[K]) Hash(h hash.Hasher[K, string]) string {
	return fmt.Sprint(h.Hash("r" + r.Dur.String()))
}

// Hash implements [TimeValue]. The material is "a" and T in RFC3339Nano,
// exact to the nanosecond.
func (a AbsoluteTime[K]) Hash(h hash.Hasher[K, string]) string {
	return fmt.Sprint(h.Hash("a" + a.T.Format(time.RFC3339Nano)))
}

// TimeFilter is a [TimeValue] held as one plain value. It is the fixed instant
// Abs when IsAbs is set, otherwise the span Dur before now.
type TimeFilter[K comparable] struct {
	Dur   time.Duration
	Abs   time.Time
	IsAbs bool
}

// Resolve implements [TimeValue]: Abs when IsAbs is set, otherwise the instant
// Dur before now.
func (tf TimeFilter[K]) Resolve(now time.Time) time.Time {
	if tf.IsAbs {
		return tf.Abs
	}
	return now.Add(-tf.Dur)
}

// Hash implements [TimeValue]. The material is "fa" and Abs in RFC3339Nano for
// an absolute bound, or "fr" and Dur for a relative one. So the two forms
// never share material, and neither collides with a [RelativeTime] or an
// [AbsoluteTime].
func (tf TimeFilter[K]) Hash(h hash.Hasher[K, string]) string {
	if tf.IsAbs {
		return fmt.Sprint(h.Hash("fa" + tf.Abs.Format(time.RFC3339Nano)))
	}
	return fmt.Sprint(h.Hash("fr" + tf.Dur.String()))
}

// ParseTimeValue converts a string into a TimeValue: a RelativeTime
// ("7d", "30m", "1w") or an AbsoluteTime ("2026-01-15" or RFC3339).
func ParseTimeValue[K comparable](s string) (TimeValue[K], error) {
	if s == "" {
		return nil, fmt.Errorf("%w: empty string", ErrInvalidTime)
	}

	// Relative: <digits><unit>. A date never ends in a unit letter. The count
	// has no sign. A duration is written bare, and a bare +7d does not lex as
	// one word. So no query could spell a signed count.
	if mult, ok := unitDuration(s[len(s)-1]); ok && s[0] >= '0' && s[0] <= '9' {
		n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		// A count past what a time.Duration holds would wrap negative. It
		// would resolve to a bound in the future and empty the window
		// (since:106752d). So it is refused with the unit's limit, not wrapped
		// or read as a date.
		limit := int64(math.MaxInt64 / mult)
		if (err == nil && n > limit) || errors.Is(err, strconv.ErrRange) {
			return nil, &DurationRangeError{Value: s, Max: limit, Unit: s[len(s)-1]}
		}
		if err == nil && n >= 0 {
			return RelativeTime[K]{Dur: time.Duration(n) * mult}, nil
		}
	}

	// Absolute: ISO date or RFC3339 datetime.
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return AbsoluteTime[K]{T: t}, nil
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
