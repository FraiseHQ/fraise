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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// reference is a fixed "now" so the relative-time tests are deterministic.
var reference = time.Date(2026, time.June, 14, 12, 0, 0, 0, time.UTC)

// TestIntervalOverlaps pins the comparison a recall's window and a fact's
// interval meet in. An interval is half-open, so one that ends where the
// other starts shares nothing with it. An instant is inside an interval from
// its start up to, not including, its end. A zero End is no end, and the zero
// Interval is all of time, the window a recall with no bounds draws.
func TestIntervalOverlaps(t *testing.T) {
	at := func(day int) time.Time {
		return time.Date(2023, time.May, day, 0, 0, 0, 0, time.UTC)
	}
	instant := func(day int) Interval { return Interval{Start: at(day), End: at(day)} }
	span := func(from, to int) Interval { return Interval{Start: at(from), End: at(to)} }

	tests := map[string]struct {
		a, b Interval
		want bool
	}{
		"instant inside a span":            {instant(8), span(7, 10), true},
		"instant at a span's start":        {instant(7), span(7, 10), true},
		"instant at a span's end":          {instant(10), span(7, 10), false},
		"instant before a span":            {instant(6), span(7, 10), false},
		"instant after a span":             {instant(11), span(7, 10), false},
		"the same instant":                 {instant(7), instant(7), true},
		"two instants":                     {instant(7), instant(8), false},
		"spans sharing days":               {span(7, 10), span(9, 12), true},
		"one span inside another":          {span(8, 9), span(7, 10), true},
		"spans end to start":               {span(7, 10), span(10, 12), false},
		"spans apart":                      {span(7, 8), span(10, 12), false},
		"open span and a later instant":    {Interval{Start: at(7)}, instant(20), true},
		"open span and its start":          {Interval{Start: at(7)}, instant(7), true},
		"open span and an earlier instant": {Interval{Start: at(7)}, instant(6), false},
		"open span and an earlier span":    {Interval{Start: at(7)}, span(3, 7), false},
		"open span and a span across it":   {Interval{Start: at(7)}, span(3, 8), true},
		"two open spans":                   {Interval{Start: at(7)}, Interval{Start: at(20)}, true},
		"no start and an earlier instant":  {Interval{End: at(7)}, instant(3), true},
		"no start and its end":             {Interval{End: at(7)}, instant(7), false},
		"all of time and an instant":       {Interval{}, instant(7), true},
		"all of time and a span":           {Interval{}, span(7, 10), true},
		"all of time and itself":           {Interval{}, Interval{}, true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tt.a.Overlaps(tt.b); got != tt.want {
				t.Errorf("a.Overlaps(b) = %v, want %v", got, tt.want)
			}
			if got := tt.b.Overlaps(tt.a); got != tt.want {
				t.Errorf("b.Overlaps(a) = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIntervalHull pins the smallest interval holding two others, whichever
// way round they are given: the earlier start and the later end. An end one
// of them lacks is an end the hull lacks too.
func TestIntervalHull(t *testing.T) {
	at := func(day int) time.Time {
		return time.Date(2023, time.May, day, 0, 0, 0, 0, time.UTC)
	}
	instant := func(day int) Interval { return Interval{Start: at(day), End: at(day)} }
	span := func(from, to int) Interval { return Interval{Start: at(from), End: at(to)} }

	tests := map[string]struct {
		a, b Interval
		want Interval
	}{
		"two instants":               {instant(7), instant(9), span(7, 9)},
		"the same instant":           {instant(7), instant(7), instant(7)},
		"instant inside a span":      {instant(8), span(7, 10), span(7, 10)},
		"instant before a span":      {instant(3), span(7, 10), span(3, 10)},
		"instant after a span":       {instant(12), span(7, 10), span(7, 12)},
		"spans sharing days":         {span(7, 10), span(9, 12), span(7, 12)},
		"spans apart":                {span(7, 8), span(10, 12), span(7, 12)},
		"one span inside another":    {span(8, 9), span(7, 10), span(7, 10)},
		"open span and an earlier":   {Interval{Start: at(7)}, span(3, 5), Interval{Start: at(3)}},
		"open span and a later":      {Interval{Start: at(7)}, span(9, 12), Interval{Start: at(7)}},
		"open span and an instant":   {Interval{Start: at(7)}, instant(20), Interval{Start: at(7)}},
		"two open spans":             {Interval{Start: at(7)}, Interval{Start: at(9)}, Interval{Start: at(7)}},
		"all of time and an instant": {Interval{}, instant(7), Interval{}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.a.Hull(tt.b)); diff != "" {
				t.Errorf("a.Hull(b) mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.want, tt.b.Hull(tt.a)); diff != "" {
				t.Errorf("b.Hull(a) mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestParseClock pins the three names a query spells after when:, and that
// each clock reads back as the name it was parsed from.
func TestParseClock(t *testing.T) {
	tests := map[string]Clock{
		"happened": Happened,
		"said":     Said,
		"written":  Written,
	}

	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			got, err := ParseClock(in)
			if err != nil {
				t.Fatalf("ParseClock(%q) unexpected error: %v", in, err)
			}
			if got != want {
				t.Errorf("ParseClock(%q) = %v, want %v", in, got, want)
			}
			if got.String() != in {
				t.Errorf("String() = %q, want %q", got.String(), in)
			}
		})
	}
}

// TestParseClockErrors pins what is not a clock: names are lower case and
// written in full, and occurred is the clause that fills happened, not its
// name.
func TestParseClockErrors(t *testing.T) {
	tests := map[string]struct{}{
		"":         {},
		"Said":     {},
		"HAPPENED": {},
		"occurred": {},
		"wrote":    {},
		"happen":   {},
	}

	for in := range tests {
		t.Run(in, func(t *testing.T) {
			got, err := ParseClock(in)
			if !errors.Is(err, ErrInvalidClock) {
				t.Fatalf("ParseClock(%q) = %v, %v; want ErrInvalidClock", in, got, err)
			}
		})
	}
}

func TestRelativeTimeResolve(t *testing.T) {
	tests := []struct {
		name string
		dur  time.Duration
		now  time.Time
		want time.Time
	}{
		{
			name: "one hour ago",
			dur:  time.Hour,
			now:  reference,
			want: reference.Add(-time.Hour),
		},
		{
			name: "thirty minutes ago",
			dur:  30 * time.Minute,
			now:  reference,
			want: reference.Add(-30 * time.Minute),
		},
		{
			name: "zero duration resolves to now",
			dur:  0,
			now:  reference,
			want: reference,
		},
		{
			name: "negative duration resolves to the future",
			dur:  -time.Hour,
			now:  reference,
			want: reference.Add(time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RelativeTime[string]{Dur: tt.dur}
			want := Interval{Start: tt.want, End: tt.want}
			if diff := cmp.Diff(want, r.Resolve(tt.now)); diff != "" {
				t.Errorf("Resolve(%v) mismatch (-want +got):\n%s", tt.now, diff)
			}
		})
	}
}

// TestAbsoluteTimeResolve pins what a literal of each precision names: its
// instant, or the whole day, month or year that starts at T. The end is the
// start of the next one, so a month ends where the calendar says it does,
// whatever its length. Resolve ignores now entirely.
func TestAbsoluteTimeResolve(t *testing.T) {
	date := func(year int, month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	instant := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)

	tests := map[string]struct {
		value AbsoluteTime[string]
		want  Interval
	}{
		"instant": {
			value: AbsoluteTime[string]{T: instant},
			want:  Interval{Start: instant, End: instant},
		},
		"day": {
			value: AbsoluteTime[string]{T: date(2023, time.May, 7), Precision: Day},
			want:  Interval{Start: date(2023, time.May, 7), End: date(2023, time.May, 8)},
		},
		"last day of a month": {
			value: AbsoluteTime[string]{T: date(2023, time.May, 31), Precision: Day},
			want:  Interval{Start: date(2023, time.May, 31), End: date(2023, time.June, 1)},
		},
		"month": {
			value: AbsoluteTime[string]{T: date(2023, time.May, 1), Precision: Month},
			want:  Interval{Start: date(2023, time.May, 1), End: date(2023, time.June, 1)},
		},
		"february of a leap year": {
			value: AbsoluteTime[string]{T: date(2024, time.February, 1), Precision: Month},
			want:  Interval{Start: date(2024, time.February, 1), End: date(2024, time.March, 1)},
		},
		"december": {
			value: AbsoluteTime[string]{T: date(2023, time.December, 1), Precision: Month},
			want:  Interval{Start: date(2023, time.December, 1), End: date(2024, time.January, 1)},
		},
		"year": {
			value: AbsoluteTime[string]{T: date(2023, time.January, 1), Precision: Year},
			want:  Interval{Start: date(2023, time.January, 1), End: date(2024, time.January, 1)},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for _, now := range []time.Time{reference, tt.value.T, {}} {
				if diff := cmp.Diff(tt.want, tt.value.Resolve(now)); diff != "" {
					t.Errorf("Resolve(%v) mismatch (-want +got):\n%s", now, diff)
				}
			}
		})
	}
}

// TestRangeTimeResolve pins what a range names: from where its first value
// starts to where its second ends, so both ends are inside it at whatever
// precision each was written. An open end resolves to no end.
func TestRangeTimeResolve(t *testing.T) {
	date := func(year int, month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	day := func(d int) AbsoluteTime[string] {
		return AbsoluteTime[string]{T: date(2023, time.May, d), Precision: Day}
	}
	instant := time.Date(2023, time.May, 7, 10, 30, 0, 0, time.UTC)

	tests := map[string]struct {
		value RangeTime[string]
		want  Interval
	}{
		"two days": {
			value: RangeTime[string]{From: day(7), To: day(9)},
			want:  Interval{Start: date(2023, time.May, 7), End: date(2023, time.May, 10)},
		},
		"the same day twice": {
			value: RangeTime[string]{From: day(7), To: day(7)},
			want:  Interval{Start: date(2023, time.May, 7), End: date(2023, time.May, 8)},
		},
		"a day to a month": {
			value: RangeTime[string]{From: day(7), To: AbsoluteTime[string]{T: date(2023, time.June, 1), Precision: Month}},
			want:  Interval{Start: date(2023, time.May, 7), End: date(2023, time.July, 1)},
		},
		"a year to an instant": {
			value: RangeTime[string]{From: AbsoluteTime[string]{T: date(2022, time.January, 1), Precision: Year}, To: AbsoluteTime[string]{T: instant}},
			want:  Interval{Start: date(2022, time.January, 1), End: instant},
		},
		"durations move with now": {
			value: RangeTime[string]{From: RelativeTime[string]{Dur: 2 * time.Hour}, To: RelativeTime[string]{Dur: time.Hour}},
			want:  Interval{Start: reference.Add(-2 * time.Hour), End: reference.Add(-time.Hour)},
		},
		"open end": {
			value: RangeTime[string]{From: day(7)},
			want:  Interval{Start: date(2023, time.May, 7)},
		},
		"open start": {
			value: RangeTime[string]{To: day(9)},
			want:  Interval{End: date(2023, time.May, 10)},
		},
		"open on both sides": {
			value: RangeTime[string]{},
			want:  Interval{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.value.Resolve(reference)); diff != "" {
				t.Errorf("Resolve mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAbsoluteTimeString(t *testing.T) {
	target := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	a := AbsoluteTime[string]{T: target}

	want := target.Format(time.RFC822)
	if got := a.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestTimeFilterResolve(t *testing.T) {
	abs := time.Date(2021, time.March, 4, 5, 6, 7, 0, time.UTC)

	tests := []struct {
		name   string
		filter TimeFilter[string]
		now    time.Time
		want   time.Time
	}{
		{
			name:   "absolute filter returns Abs and ignores now",
			filter: TimeFilter[string]{Abs: abs, IsAbs: true, Dur: time.Hour},
			now:    reference,
			want:   abs,
		},
		{
			name:   "relative filter subtracts duration from now",
			filter: TimeFilter[string]{Dur: 2 * time.Hour, IsAbs: false},
			now:    reference,
			want:   reference.Add(-2 * time.Hour),
		},
		{
			name:   "relative filter with zero duration returns now",
			filter: TimeFilter[string]{Dur: 0, IsAbs: false},
			now:    reference,
			want:   reference,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := Interval{Start: tt.want, End: tt.want}
			if diff := cmp.Diff(want, tt.filter.Resolve(tt.now)); diff != "" {
				t.Errorf("Resolve(%v) mismatch (-want +got):\n%s", tt.now, diff)
			}
		})
	}
}

// TestTimeValueInterface verifies that the concrete types satisfy the
// TimeValue interface and resolve consistently when used through it.
func TestTimeValueInterface(t *testing.T) {
	abs := time.Date(2022, time.July, 8, 9, 10, 11, 0, time.UTC)

	values := []struct {
		name string
		tv   TimeValue[string]
		want time.Time
	}{
		{"RelativeTime", RelativeTime[string]{Dur: time.Hour}, reference.Add(-time.Hour)},
		{"AbsoluteTime", AbsoluteTime[string]{T: abs}, abs},
		{"TimeFilter relative", TimeFilter[string]{Dur: time.Minute}, reference.Add(-time.Minute)},
		{"TimeFilter absolute", TimeFilter[string]{Abs: abs, IsAbs: true}, abs},
		{"RangeTime", RangeTime[string]{From: AbsoluteTime[string]{T: abs}, To: AbsoluteTime[string]{T: abs}}, abs},
	}

	for _, v := range values {
		t.Run(v.name, func(t *testing.T) {
			want := Interval{Start: v.want, End: v.want}
			if diff := cmp.Diff(want, v.tv.Resolve(reference)); diff != "" {
				t.Errorf("Resolve(%v) mismatch (-want +got):\n%s", reference, diff)
			}
		})
	}
}

// TestTimeValueHash pins the cache-key forms. Each implementation has a
// distinct prefix, so no two kinds of bound can render alike. A range folds
// in the rendered keys of its two ends, the first length-prefixed.
func TestTimeValueHash(t *testing.T) {
	abs := time.Date(2022, time.July, 8, 9, 10, 11, 0, time.UTC)
	day := AbsoluteTime[string]{T: time.Date(2022, time.July, 8, 0, 0, 0, 0, time.UTC), Precision: Day}
	month := AbsoluteTime[string]{T: time.Date(2022, time.July, 1, 0, 0, 0, 0, time.UTC), Precision: Month}
	year := AbsoluteTime[string]{T: time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC), Precision: Year}

	tests := []struct {
		name string
		tv   TimeValue[string]
		want string
	}{
		{"RelativeTime", RelativeTime[string]{Dur: time.Hour}, "H(r1h0m0s)"},
		{"AbsoluteTime", AbsoluteTime[string]{T: abs}, "H(a2022-07-08T09:10:11Z)"},
		{"AbsoluteTime day", day, "H(ad2022-07-08T00:00:00Z)"},
		{"AbsoluteTime month", month, "H(am2022-07-01T00:00:00Z)"},
		{"AbsoluteTime year", year, "H(ay2022-01-01T00:00:00Z)"},
		{"RangeTime", RangeTime[string]{From: year, To: day}, "H(s25:H(ay2022-01-01T00:00:00Z)H(ad2022-07-08T00:00:00Z))"},
		{"RangeTime open end", RangeTime[string]{From: day}, "H(s25:H(ad2022-07-08T00:00:00Z))"},
		{"RangeTime open start", RangeTime[string]{To: day}, "H(s0:H(ad2022-07-08T00:00:00Z))"},
		{"TimeFilter relative", TimeFilter[string]{Dur: time.Minute}, "H(fr1m0s)"},
		{"TimeFilter absolute", TimeFilter[string]{Abs: abs, IsAbs: true}, "H(fa2022-07-08T09:10:11Z)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tv.Hash(fakeHasher{}); got != tt.want {
				t.Errorf("Hash() = %q, want %q", got, tt.want)
			}
		})
	}

	// String() is RFC822, with minute precision. The hash material keeps full
	// precision, so two absolute bounds a second apart must not collide.
	a := AbsoluteTime[string]{T: abs}
	b := AbsoluteTime[string]{T: abs.Add(time.Second)}
	if a.Hash(fakeHasher{}) == b.Hash(fakeHasher{}) {
		t.Errorf("Hash collision: bounds a second apart both produced %q", a.Hash(fakeHasher{}))
	}
}

// TestTimeValueHashDistinguishes proves that values resolving to different
// intervals never share a key. '2023-01-01', '2023-01' and '2023' all start
// at the same instant, and a range shares its ends with its open form and
// with the range written the other way round. A recall keyed alike for two
// of them would run in the other's window.
func TestTimeValueHashDistinguishes(t *testing.T) {
	start := time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)
	instant := AbsoluteTime[string]{T: start}
	day := AbsoluteTime[string]{T: start, Precision: Day}
	month := AbsoluteTime[string]{T: start, Precision: Month}
	year := AbsoluteTime[string]{T: start, Precision: Year}

	variants := map[string]TimeValue[string]{
		"instant":             instant,
		"day":                 day,
		"month":               month,
		"year":                year,
		"day to month":        RangeTime[string]{From: day, To: month},
		"month to day":        RangeTime[string]{From: month, To: day},
		"day, open end":       RangeTime[string]{From: day},
		"day, open start":     RangeTime[string]{To: day},
		"day to itself":       RangeTime[string]{From: day, To: day},
		"open on both sides":  RangeTime[string]{},
		"relative":            RelativeTime[string]{Dur: time.Hour},
		"range of one filter": RangeTime[string]{From: TimeFilter[string]{Abs: start, IsAbs: true}},
	}

	seen := make(map[string]string, len(variants))
	for name, value := range variants {
		got := value.Hash(fakeHasher{})
		if other, dup := seen[got]; dup {
			t.Errorf("%q and %q share the key %q", name, other, got)
		}
		seen[got] = name
	}
}

// TestTimeValueEqual pins the contract the plan cache relies on to confirm a
// hit. Bounds of a different type, span, instant or precision are never equal,
// even where they resolve alike today, since their material differs. The same
// instant written in two zones is equal, because it bounds the same window.
func TestTimeValueEqual(t *testing.T) {
	abs := time.Date(2026, time.June, 18, 12, 0, 0, 0, time.UTC)
	day := AbsoluteTime[string]{T: abs, Precision: Day}
	tests := map[string]struct {
		a, b TimeValue[string]
		want bool
	}{
		"relative same":            {RelativeTime[string]{Dur: time.Hour}, RelativeTime[string]{Dur: time.Hour}, true},
		"relative other span":      {RelativeTime[string]{Dur: time.Hour}, RelativeTime[string]{Dur: time.Minute}, false},
		"absolute same":            {AbsoluteTime[string]{T: abs}, AbsoluteTime[string]{T: abs}, true},
		"absolute other zone":      {AbsoluteTime[string]{T: abs}, AbsoluteTime[string]{T: abs.In(time.FixedZone("CEST", 2*60*60))}, true},
		"absolute other instant":   {AbsoluteTime[string]{T: abs}, AbsoluteTime[string]{T: abs.Add(time.Nanosecond)}, false},
		"relative vs absolute":     {RelativeTime[string]{Dur: time.Hour}, AbsoluteTime[string]{T: abs}, false},
		"relative vs filter":       {RelativeTime[string]{Dur: time.Hour}, TimeFilter[string]{Dur: time.Hour}, false},
		"filter relative same":     {TimeFilter[string]{Dur: time.Hour, Abs: abs}, TimeFilter[string]{Dur: time.Hour}, true},
		"filter absolute same":     {TimeFilter[string]{Abs: abs, IsAbs: true, Dur: time.Hour}, TimeFilter[string]{Abs: abs, IsAbs: true}, true},
		"filter other form":        {TimeFilter[string]{Dur: time.Hour}, TimeFilter[string]{Dur: time.Hour, IsAbs: true}, false},
		"filter other span":        {TimeFilter[string]{Dur: time.Hour}, TimeFilter[string]{Dur: time.Minute}, false},
		"filter other instant":     {TimeFilter[string]{Abs: abs, IsAbs: true}, TimeFilter[string]{Abs: abs.Add(time.Second), IsAbs: true}, false},
		"absolute same day":        {day, AbsoluteTime[string]{T: abs, Precision: Day}, true},
		"absolute other precision": {day, AbsoluteTime[string]{T: abs, Precision: Month}, false},
		"instant vs day":           {AbsoluteTime[string]{T: abs}, day, false},
		"range same":               {RangeTime[string]{From: day, To: day}, RangeTime[string]{From: day, To: day}, true},
		"range other end":          {RangeTime[string]{From: day, To: day}, RangeTime[string]{From: day, To: AbsoluteTime[string]{T: abs}}, false},
		"range other start":        {RangeTime[string]{From: day, To: day}, RangeTime[string]{From: AbsoluteTime[string]{T: abs}, To: day}, false},
		"range open same":          {RangeTime[string]{From: day}, RangeTime[string]{From: day}, true},
		"range open vs closed":     {RangeTime[string]{From: day}, RangeTime[string]{From: day, To: day}, false},
		"range open start vs end":  {RangeTime[string]{From: day}, RangeTime[string]{To: day}, false},
		"range vs its start":       {RangeTime[string]{From: day}, day, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("a.Equal(b) = %v, want %v", got, tt.want)
			}
			if got := tt.b.Equal(tt.a); got != tt.want {
				t.Errorf("b.Equal(a) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTimeValueRelative(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"45s", 45 * time.Second},
		{"30m", 30 * time.Minute},
		{"2h", 2 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"1w", 7 * 24 * time.Hour},
		{"0d", 0}, // zero is allowed
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseTimeValue[string](tt.in)
			if err != nil {
				t.Fatalf("ParseTimeValue(%q) unexpected error: %v", tt.in, err)
			}
			rel, ok := got.(RelativeTime[string])
			if !ok {
				t.Fatalf("ParseTimeValue(%q) = %T, want RelativeTime", tt.in, got)
			}
			if rel.Dur != tt.want {
				t.Errorf("ParseTimeValue(%q).Dur = %v, want %v", tt.in, rel.Dur, tt.want)
			}
		})
	}
}

// TestParseTimeValueAbsolute pins the precision a literal carries: the one it
// was written at. A year is four digits and a month two, so a shorter form is
// never read as a truncated longer one.
func TestParseTimeValueAbsolute(t *testing.T) {
	tests := map[string]AbsoluteTime[string]{
		"2026-01-15":                {T: time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC), Precision: Day},
		"2026-01":                   {T: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), Precision: Month},
		"2026":                      {T: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), Precision: Year},
		"2026-01-15T10:30:00Z":      {T: time.Date(2026, time.January, 15, 10, 30, 0, 0, time.UTC)},
		"2026-01-15T10:30:00+02:00": {T: time.Date(2026, time.January, 15, 8, 30, 0, 0, time.UTC)},
		"2026-01-15T00:00:00Z":      {T: time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)},
	}

	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			got, err := ParseTimeValue[string](in)
			if err != nil {
				t.Fatalf("ParseTimeValue(%q) unexpected error: %v", in, err)
			}
			if diff := cmp.Diff(TimeValue[string](want), got); diff != "" {
				t.Errorf("ParseTimeValue(%q) mismatch (-want +got):\n%s", in, diff)
			}
		})
	}
}

func TestParseTimeValueErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"bare number, no unit", "7"},
		{"garbage", "abc"},
		{"unit suffix, non-numeric prefix", "xd"},
		{"negative relative", "-5d"},
		{"signed relative", "+7d"},
		{"unknown unit", "7x"},
		{"fractional not supported", "1.5h"},
		{"invalid date", "2026-13-40"},
		{"invalid month", "2026-13"},
		{"month of one digit", "2026-1"},
		{"year of three digits", "202"},
		{"year of five digits", "20260"},
		{"date with a trailing dash", "2026-01-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeValue[string](tt.in)
			if err == nil {
				t.Fatalf("ParseTimeValue(%q) = %#v, want error", tt.in, got)
			}
			if got != nil {
				t.Errorf("ParseTimeValue(%q) returned non-nil value %#v alongside error", tt.in, got)
			}
		})
	}
}

// TestParseTimeValueBoundsRelativeDurations pins the range of a relative
// duration. The count may reach the largest a time.Duration holds for its
// unit, and no further. Past it the product would wrap negative, and
// since:106752d would resolve to a bound in the future that empties every
// window. The error names the unit's limit and still reads as ErrInvalidTime.
func TestParseTimeValueBoundsRelativeDurations(t *testing.T) {
	tests := []struct {
		in  string
		max int64 // 0: the value is in range and must parse
	}{
		{"106751d", 0},
		{"106752d", 106751},
		{"15250w", 0},
		{"15251w", 15250},
		{"9223372036s", 0},
		{"9223372037s", 9223372036},
		{"99999999999999999999d", 106751}, // past int64 itself
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseTimeValue[string](tt.in)

			if tt.max == 0 {
				rt, ok := got.(RelativeTime[string])
				if err != nil || !ok || rt.Dur <= 0 {
					t.Fatalf("ParseTimeValue(%q) = %#v, %v; want a positive relative duration", tt.in, got, err)
				}
				return
			}

			var rangeErr *DurationRangeError
			if !errors.As(err, &rangeErr) {
				t.Fatalf("ParseTimeValue(%q) = %#v, %v; want a DurationRangeError", tt.in, got, err)
			}
			if rangeErr.Max != tt.max || rangeErr.Unit != tt.in[len(tt.in)-1] {
				t.Errorf("range = at most %d%c, want %d%c", rangeErr.Max, rangeErr.Unit, tt.max, tt.in[len(tt.in)-1])
			}
			if !errors.Is(err, ErrInvalidTime) {
				t.Errorf("error %v is not an ErrInvalidTime", err)
			}
		})
	}
}

func TestUnitDuration(t *testing.T) {
	tests := []struct {
		b    byte
		want time.Duration
		ok   bool
	}{
		{'s', time.Second, true},
		{'m', time.Minute, true},
		{'h', time.Hour, true},
		{'d', 24 * time.Hour, true},
		{'w', 7 * 24 * time.Hour, true},
		{'x', 0, false},
		{'y', 0, false},
		{'D', 0, false}, // case-sensitive
	}

	for _, tt := range tests {
		t.Run(string(tt.b), func(t *testing.T) {
			got, ok := unitDuration(tt.b)
			if got != tt.want || ok != tt.ok {
				t.Errorf("unitDuration(%q) = (%v, %v), want (%v, %v)", tt.b, got, ok, tt.want, tt.ok)
			}
		})
	}
}
