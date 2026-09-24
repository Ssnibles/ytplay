package main

import "testing"

func TestLayoutGeometry(t *testing.T) {
	cases := []struct {
		w, h                                int
		left, right, mid, avail, cols, rows int
		ok                                  bool
	}{
		{120, 40, 54, 65, 36, 34, 61, 17, true},
		{80, 24, 36, 43, 20, 18, 39, 11, true},
		{40, 12, 18, 21, 8, 6, 0, 0, false},
		{0, 0, 0, 0, 0, 0, 0, 0, false},
	}
	for _, c := range cases {
		l := computeLayout(c.w, c.h)
		if l.leftW != c.left || l.rightW != c.right || l.midH != c.mid ||
			l.avail != c.avail || l.cols != c.cols || l.rows != c.rows || l.thumbOK != c.ok {
			t.Errorf("%dx%d: got left=%d right=%d mid=%d avail=%d cols=%d rows=%d ok=%v",
				c.w, c.h, l.leftW, l.rightW, l.midH, l.avail, l.cols, l.rows, l.thumbOK)
		}
	}
}
