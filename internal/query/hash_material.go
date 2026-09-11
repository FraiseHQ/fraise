package query

import (
	"strconv"
	"strings"

	"github.com/FraiseHQ/fraise/internal/containers"
)

// writeField appends a length-prefixed field so values that contain the old
// delimiters ('|' / NUL) cannot shift field boundaries in plan-cache material.
func writeField(b *strings.Builder, name, value string) {
	b.WriteString(name)
	b.WriteByte('=')
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
	b.WriteByte('|')
}

// writeList appends a length-prefixed list of length-prefixed strings.
func writeList(b *strings.Builder, name string, values []string) {
	b.WriteString(name)
	b.WriteByte('=')
	b.WriteString(strconv.Itoa(len(values)))
	b.WriteByte('[')
	for i, value := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(len(value)))
		b.WriteByte(':')
		b.WriteString(value)
	}
	b.WriteByte(']')
	b.WriteByte('|')
}

func timeValuesEqual[K comparable](a, b containers.TimeValue[K]) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if a == nil {
		return true
	}
	switch av := a.(type) {
	case containers.RelativeTime[K]:
		bv, ok := b.(containers.RelativeTime[K])
		return ok && av.Dur == bv.Dur
	case *containers.RelativeTime[K]:
		bv, ok := b.(*containers.RelativeTime[K])
		return ok && av.Dur == bv.Dur
	case containers.AbsoluteTime[K]:
		bv, ok := b.(containers.AbsoluteTime[K])
		return ok && av.T.Equal(bv.T)
	case *containers.AbsoluteTime[K]:
		bv, ok := b.(*containers.AbsoluteTime[K])
		return ok && av.T.Equal(bv.T)
	default:
		return false
	}
}
