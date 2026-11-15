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
	"strings"
	"time"

	"github.com/FraiseHQ/fraise/internal/hash"
)

// Interval is a span of time, half-open: it holds Start and every instant up
// to End, End itself excluded. An interval whose End equals its Start is that
// one instant. One whose End is zero has no end, so the zero Interval is all
// of time. A fact holds intervals for when what it states happened, and every
// [TimeValue] resolves to one, so a bound and a fact compare by overlap
// whatever precision each was written at.
type Interval struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end,omitzero"`
}

// Overlaps reports whether i and o share an instant. A recall's window and a
// fact's interval are compared with it, so a fact that happened over a month
// is inside every window that takes in part of that month.
func (i Interval) Overlaps(o Interval) bool {
	return i.startsWithin(o) && o.startsWithin(i)
}

// startsWithin reports whether i has begun by the time o is over: always when
// o has no end, at or before o's instant when o is one, and strictly before
// o's End otherwise, since End itself is outside o.
func (i Interval) startsWithin(o Interval) bool {
	switch {
	case o.End.IsZero():
		return true
	case o.End.Equal(o.Start):
		return !i.Start.After(o.Start)
	default:
		return i.Start.Before(o.End)
	}
}

// Hull returns the smallest interval holding both i and o: from the earlier
// Start to the later End, and without an end when either has none. A when
// over several facts answers with the hull of what they hold.
func (i Interval) Hull(o Interval) Interval {
	hull := i
	if o.Start.Before(hull.Start) {
		hull.Start = o.Start
	}
	switch {
	case hull.End.IsZero() || o.End.IsZero():
		hull.End = time.Time{}
	case o.End.After(hull.End):
		hull.End = o.End
	}
	return hull
}

// Clock names one of the three times a fact carries: when what it states
// happened, when it was said, and when the server wrote it. A fact given no
// time of its own reads the next one down, happened then said then written,
// so a fact written with no times reads the same on all three.
type Clock uint8

const (
	// Happened is when what the fact states took place. It is the zero value,
	// the clock a bound reads unless it names another.
	Happened Clock = iota
	// Said is when the fact was stated, the date of the message it came from.
	Said
	// Written is when the server stored the fact.
	Written
)

// String returns the clock's name as a query spells it after when:.
func (c Clock) String() string {
	switch c {
	case Said:
		return "said"
	case Written:
		return "written"
	default:
		return "happened"
	}
}

// ParseClock reads a clock's name as a query spells it after when:.
func ParseClock(s string) (Clock, error) {
	for _, c := range []Clock{Happened, Said, Written} {
		if s == c.String() {
			return c, nil
		}
	}
	return Happened, fmt.Errorf("%w: %q (want said, happened or written)", ErrInvalidClock, s)
}

// Bound is one edge of a recall's time window as a graph receives it: the
// instant At, compared with what each fact holds on Clock. A zero At is no
// bound.
type Bound struct {
	At    time.Time
	Clock Clock
}

// TimeValue is a time a query names: a bound of a recall's window, or one of
// the times a remember gives its fact. It resolves itself against now to the
// interval it names. It hashes itself with the same Hasher[K, string] as its
// enclosing query, the way Recall and Remember do.
type TimeValue[K comparable] interface {
	Resolve(now time.Time) Interval
	// Hash keys the value through h and renders the key for folding into an
	// enclosing query's hash material. The material is lossless: a duration as
	// Duration.String renders it, an instant in RFC3339Nano. Each type adds a
	// distinct prefix, so different values never share material. If two did, a
	// recall would reuse the plan cached for another time window. String()
	// cannot serve as material, because AbsoluteTime's RFC822 form drops
	// seconds.
	Hash(h hash.Hasher[K, string]) string
	// Equal reports whether other is the same value: the same type, with the
	// same span or the same instant at the same precision. The engine confirms
	// a plan-cache hit with it, because two values may still share a key when
	// their material does not. Equal must never hold for values that resolve
	// differently, or a recall would run in another query's time window.
	Equal(other TimeValue[K]) bool
}

// RelativeTime is a [TimeValue] a fixed span before now, as in since:7d. It
// moves with the clock, so the same query resolves to a later instant each
// time it runs.
type RelativeTime[K comparable] struct{ Dur time.Duration }

// Precision is how much of the calendar a date literal names. A literal
// resolves to the whole of what it names: '2023-05-07' to that day, '2023-05'
// to that month, '2023' to that year, and a full timestamp to its instant.
type Precision uint8

const (
	// Instant is a full timestamp. It is the zero value, so an AbsoluteTime
	// built from a time alone is that one instant.
	Instant Precision = iota
	// Day is a calendar date, 2023-05-07.
	Day
	// Month is a calendar month, 2023-05.
	Month
	// Year is a calendar year, 2023.
	Year
)

// AbsoluteTime is a [TimeValue] fixed on the calendar, as in
// since:'2026-01-15'. T is where it starts and Precision how far it runs: an
// instant, or the day, month or year starting at T. It resolves to the same
// interval whenever the query runs.
type AbsoluteTime[K comparable] struct {
	T         time.Time
	Precision Precision
}

func (r RelativeTime[K]) String() string {
	return r.Dur.String()
}

func (a AbsoluteTime[K]) String() string {
	return a.T.Format(time.RFC822)
}

// Resolve implements [TimeValue]: the instant Dur before now.
func (r RelativeTime[K]) Resolve(now time.Time) Interval {
	at := now.Add(-r.Dur)
	return Interval{Start: at, End: at}
}

// Resolve implements [TimeValue]: the instant T, or the day, month or year
// that starts at T, whatever now is.
func (a AbsoluteTime[K]) Resolve(_ time.Time) Interval {
	switch a.Precision {
	case Day:
		return Interval{Start: a.T, End: a.T.AddDate(0, 0, 1)}
	case Month:
		return Interval{Start: a.T, End: a.T.AddDate(0, 1, 0)}
	case Year:
		return Interval{Start: a.T, End: a.T.AddDate(1, 0, 0)}
	default:
		return Interval{Start: a.T, End: a.T}
	}
}

// Hash implements [TimeValue]. The material is "r" and the duration. No other
// value's material starts with "r".
func (r RelativeTime[K]) Hash(h hash.Hasher[K, string]) string {
	return fmt.Sprint(h.Hash("r" + r.Dur.String()))
}

// Hash implements [TimeValue]. The material is "a", a letter for a precision
// wider than an instant (d, m or y), and T in RFC3339Nano, exact to the
// nanosecond. The letter keeps '2023-01-01' apart from '2023-01' and '2023',
// which all start at the same T and resolve to different intervals.
func (a AbsoluteTime[K]) Hash(h hash.Hasher[K, string]) string {
	var precision string
	switch a.Precision {
	case Day:
		precision = "d"
	case Month:
		precision = "m"
	case Year:
		precision = "y"
	}
	return fmt.Sprint(h.Hash("a" + precision + a.T.Format(time.RFC3339Nano)))
}

// Equal implements [TimeValue]: other is a RelativeTime of the same span.
func (r RelativeTime[K]) Equal(other TimeValue[K]) bool {
	o, ok := other.(RelativeTime[K])
	return ok && o.Dur == r.Dur
}

// Equal implements [TimeValue]: other is an AbsoluteTime at the same instant,
// whatever zone each was written in, and of the same precision.
func (a AbsoluteTime[K]) Equal(other TimeValue[K]) bool {
	o, ok := other.(AbsoluteTime[K])
	return ok && o.T.Equal(a.T) && o.Precision == a.Precision
}

// RangeTime is a [TimeValue] running from one value to another, as in
// '2023-05-07'..'2023-05-09': from the start of From to the end of To, so
// both days are inside it. A nil To leaves it without an end, as in
// '2023-05-07'.., for what began and is not over. A nil From starts it at
// the beginning of time.
type RangeTime[K comparable] struct {
	From TimeValue[K]
	To   TimeValue[K]
}

// Resolve implements [TimeValue]: from where From starts to where To ends,
// each resolved against the same now.
func (r RangeTime[K]) Resolve(now time.Time) Interval {
	var span Interval
	if r.From != nil {
		span.Start = r.From.Resolve(now).Start
	}
	if r.To != nil {
		span.End = r.To.Resolve(now).End
	}
	return span
}

// Hash implements [TimeValue]. The material is "s", then the rendered key of
// From, length-prefixed, then that of To. The length keeps the two ends
// apart whatever a key renders as, and an open end contributes nothing, so
// 'a'.. and 'a'..'b' never share material.
func (r RangeTime[K]) Hash(h hash.Hasher[K, string]) string {
	var from, to string
	if r.From != nil {
		from = r.From.Hash(h)
	}
	if r.To != nil {
		to = r.To.Hash(h)
	}
	var b strings.Builder
	b.WriteByte('s')
	b.WriteString(strconv.Itoa(len(from)))
	b.WriteByte(':')
	b.WriteString(from)
	b.WriteString(to)
	return fmt.Sprint(h.Hash(b.String()))
}

// Equal implements [TimeValue]: other is a RangeTime whose two ends are equal
// to this one's, an open end matching only an open end.
func (r RangeTime[K]) Equal(other TimeValue[K]) bool {
	o, ok := other.(RangeTime[K])
	if !ok || (r.From == nil) != (o.From == nil) || (r.To == nil) != (o.To == nil) {
		return false
	}
	return (r.From == nil || r.From.Equal(o.From)) && (r.To == nil || r.To.Equal(o.To))
}

// TimeFilter is a [TimeValue] held as one plain value. It is the fixed instant
// Abs when IsAbs is set, otherwise the span Dur before now.
type TimeFilter[K comparable] struct {
	Dur   time.Duration
	Abs   time.Time
	IsAbs bool
}

// Resolve implements [TimeValue]: the instant Abs when IsAbs is set, otherwise
// the instant Dur before now.
func (tf TimeFilter[K]) Resolve(now time.Time) Interval {
	at := now.Add(-tf.Dur)
	if tf.IsAbs {
		at = tf.Abs
	}
	return Interval{Start: at, End: at}
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

// Equal implements [TimeValue]: other is a TimeFilter of the same form, at the
// same instant when absolute or of the same span when relative. The field the
// form ignores does not count.
func (tf TimeFilter[K]) Equal(other TimeValue[K]) bool {
	o, ok := other.(TimeFilter[K])
	if !ok || o.IsAbs != tf.IsAbs {
		return false
	}
	if tf.IsAbs {
		return o.Abs.Equal(tf.Abs)
	}
	return o.Dur == tf.Dur
}

// ParseTimeValue converts a string into a TimeValue: a RelativeTime
// ("7d", "30m", "1w") or an AbsoluteTime. An absolute value carries the
// precision it was written at: a day ("2026-01-15"), a month ("2026-01"), a
// year ("2026") or an RFC3339 instant.
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

	// Absolute: an ISO date, month or year, or an RFC3339 datetime. Each
	// layout is tried whole, so "2026" is never a truncated date.
	for _, form := range []struct {
		layout    string
		precision Precision
	}{
		{"2006-01-02", Day},
		{"2006-01", Month},
		{"2006", Year},
		{time.RFC3339, Instant},
	} {
		if t, err := time.Parse(form.layout, s); err == nil {
			return AbsoluteTime[K]{T: t, Precision: form.precision}, nil
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
