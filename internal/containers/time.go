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

// TimeValue is a time bound that can resolve itself against "now" and hash
// itself with the same Hasher[K, string] its enclosing query uses, mirroring
// the Recall/Remember pattern.
type TimeValue[K comparable] interface {
	Resolve(now time.Time) time.Time
	// Hash keys the bound through h and renders the key for folding into an
	// enclosing query's hash material. The material is lossless (a duration as
	// Duration.String renders it, an instant in RFC3339Nano) and each
	// implementation prefixes it distinctly, so different bounds never share
	// it; if two did, a recall would reuse the plan cached for another time
	// window. String() cannot serve as material: AbsoluteTime's RFC822 form
	// drops seconds.
	Hash(h hash.Hasher[K, string]) string
}

// RelativeTime is a [TimeValue] a fixed span before now, as in since:7d. It
// moves with the clock: the same query resolves to a later instant each time
// it runs.
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

// Hash implements [TimeValue]. The material is "r" and the duration, a prefix
// no other bound's material starts with.
func (r RelativeTime[K]) Hash(h hash.Hasher[K, string]) string {
	return fmt.Sprint(h.Hash("r" + r.Dur.String()))
}

// Hash implements [TimeValue]. The material is "a" and T in RFC3339Nano,
// exact to the nanosecond, so instants a second apart never share a cached
// plan the way their RFC822 String forms would.
func (a AbsoluteTime[K]) Hash(h hash.Hasher[K, string]) string {
	return fmt.Sprint(h.Hash("a" + a.T.Format(time.RFC3339Nano)))
}

// TimeFilter is a time bound held as one plain value instead of behind the
// [TimeValue] interface: IsAbs selects whether the fixed instant Abs or the
// span Dur before now applies, and the other field is ignored. Its method set
// is TimeValue's.
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

// Hash implements [TimeValue]. The material is prefixed "fa" for an absolute
// bound (RFC3339Nano) and "fr" for a relative one, so the two forms never
// share it, and neither collides with a [RelativeTime] or [AbsoluteTime].
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

	// Relative: <int><unit>. A date never ends in a unit letter.
	if mult, ok := unitDuration(s[len(s)-1]); ok {
		n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		// A count past what a time.Duration holds would wrap negative and
		// resolve to a bound in the future, emptying the window (since:106752d),
		// so it is refused with the unit's limit rather than wrapped or read as
		// a date.
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
