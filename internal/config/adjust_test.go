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

import (
	"testing"
	"time"
)

// adjustCase verifies Adjust for a single type T.
func adjustCase[T comparable](t *testing.T, name string, value, defValue, want T) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		v := value
		Adjust(&v, defValue)
		if v != want {
			t.Errorf("Adjust(%v, %v) = %v, want %v", value, defValue, v, want)
		}
	})
}

func TestAdjustString(t *testing.T) {
	adjustCase(t, "empty uses default", "", "info", "info")
	adjustCase(t, "set value is kept", "debug", "info", "debug")
	adjustCase(t, "empty default on empty value", "", "", "")
}

func TestAdjustInt(t *testing.T) {
	adjustCase(t, "zero uses default", 0, 8, 8)
	adjustCase(t, "set value is kept", 4, 8, 4)
	adjustCase(t, "negative value is kept", -1, 8, -1)
}

func TestAdjustUint(t *testing.T) {
	adjustCase[uint](t, "zero uses default", 0, 8, 8)
	adjustCase[uint](t, "set value is kept", 4, 8, 4)
}

func TestAdjustBool(t *testing.T) {
	adjustCase(t, "false uses default true", false, true, true)
	adjustCase(t, "false keeps default false", false, false, false)
	adjustCase(t, "true is kept over default false", true, false, true)
	adjustCase(t, "true is kept over default true", true, true, true)
}

func TestAdjustFloat32(t *testing.T) {
	adjustCase[float32](t, "zero uses default", 0, 0.5, 0.5)
	adjustCase[float32](t, "set value is kept", 0.25, 0.5, 0.25)
	adjustCase[float32](t, "negative value is kept", -1, 0.5, -1)
}

func TestAdjustDuration(t *testing.T) {
	adjustCase(t, "zero uses default", time.Duration(0), time.Hour, time.Hour)
	adjustCase(t, "set value is kept", time.Minute, time.Hour, time.Minute)
	adjustCase(t, "negative value is kept", -time.Second, time.Hour, -time.Second)
}
