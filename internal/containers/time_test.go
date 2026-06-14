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
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/query/parser"
)

// reference is a fixed "now" so the relative-time tests are deterministic.
var reference = time.Date(2026, time.June, 14, 12, 0, 0, 0, time.UTC)

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
			r := parser.RelativeTime{Dur: tt.dur}
			if got := r.Resolve(tt.now); !got.Equal(tt.want) {
				t.Errorf("Resolve(%v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

func TestAbsoluteTimeResolve(t *testing.T) {
	target := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	a := parser.AbsoluteTime{T: target}

	// Resolve must ignore "now" entirely and return the wrapped instant.
	for _, now := range []time.Time{reference, target, time.Time{}} {
		if got := a.Resolve(now); !got.Equal(target) {
			t.Errorf("Resolve(%v) = %v, want %v", now, got, target)
		}
	}
}

func TestAbsoluteTimeString(t *testing.T) {
	target := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	a := parser.AbsoluteTime{T: target}

	want := target.Format(time.RFC822)
	if got := a.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestTimeFilterResolve(t *testing.T) {
	abs := time.Date(2021, time.March, 4, 5, 6, 7, 0, time.UTC)

	tests := []struct {
		name   string
		filter parser.TimeFilter
		now    time.Time
		want   time.Time
	}{
		{
			name:   "absolute filter returns Abs and ignores now",
			filter: parser.TimeFilter{Abs: abs, IsAbs: true, Dur: time.Hour},
			now:    reference,
			want:   abs,
		},
		{
			name:   "relative filter subtracts duration from now",
			filter: parser.TimeFilter{Dur: 2 * time.Hour, IsAbs: false},
			now:    reference,
			want:   reference.Add(-2 * time.Hour),
		},
		{
			name:   "relative filter with zero duration returns now",
			filter: parser.TimeFilter{Dur: 0, IsAbs: false},
			now:    reference,
			want:   reference,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Resolve(tt.now); !got.Equal(tt.want) {
				t.Errorf("Resolve(%v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

// TestTimeValueInterface verifies that all three concrete types satisfy the
// TimeValue interface and resolve consistently when used through it.
func TestTimeValueInterface(t *testing.T) {
	abs := time.Date(2022, time.July, 8, 9, 10, 11, 0, time.UTC)

	values := []struct {
		name string
		tv   parser.TimeValue
		want time.Time
	}{
		{"RelativeTime", parser.RelativeTime{Dur: time.Hour}, reference.Add(-time.Hour)},
		{"AbsoluteTime", parser.AbsoluteTime{T: abs}, abs},
		{"TimeFilter relative", parser.TimeFilter{Dur: time.Minute}, reference.Add(-time.Minute)},
		{"TimeFilter absolute", parser.TimeFilter{Abs: abs, IsAbs: true}, abs},
	}

	for _, v := range values {
		t.Run(v.name, func(t *testing.T) {
			if got := v.tv.Resolve(reference); !got.Equal(v.want) {
				t.Errorf("Resolve(%v) = %v, want %v", reference, got, v.want)
			}
		})
	}
}
