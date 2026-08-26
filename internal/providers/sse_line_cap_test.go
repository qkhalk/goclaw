package providers

import (
	"strings"
	"testing"
)

// A3: a single SSE line longer than SSELineMaxBytes must stop the scanner
// with ErrSSELineTooLarge instead of buffering without bound.
func TestSSEScanner_LineOverCapFails(t *testing.T) {
	huge := strings.Repeat("x", SSELineMaxBytes+1024)
	input := "data: " + huge + "\n"
	sc := NewSSEScanner(strings.NewReader(input))

	for sc.Next() {
		// drain; a legit stream would terminate quickly, but this one must
		// abort at the cap.
	}
	if err := sc.Err(); err == nil {
		t.Fatal("expected ErrSSELineTooLarge, got nil")
	} else if err != ErrSSELineTooLarge {
		t.Fatalf("err = %v, want ErrSSELineTooLarge", err)
	}
}

// A frame that fits under the cap (multi-MB base64 image case) still works.
func TestSSEScanner_LargeButLegalFrameOK(t *testing.T) {
	payload := strings.Repeat("a", 4<<20) // 4MB single data line — legal
	input := "data: " + payload + "\n"
	sc := NewSSEScanner(strings.NewReader(input))

	if !sc.Next() {
		t.Fatalf("Next() = false for 4MB line, want true; err=%v", sc.Err())
	}
	if len(sc.Data()) != len(payload) {
		t.Fatalf("data length = %d, want %d", len(sc.Data()), len(payload))
	}
	if sc.Next() {
		t.Fatal("unexpected extra frame")
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

// The cap is per-line: many small lines never trip it.
func TestSSEScanner_ManySmallLinesOK(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 10_000; i++ {
		b.WriteString("data: {\"i\":")
		b.WriteString(strings.Repeat("0", 100))
		b.WriteString("}\n\n")
	}
	sc := NewSSEScanner(strings.NewReader(b.String()))
	n := 0
	for sc.Next() {
		n++
	}
	if n != 10_000 {
		t.Fatalf("frames = %d, want 10000", n)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}
