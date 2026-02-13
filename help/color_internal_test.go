package help

import (
	"os"
	"testing"
)

func TestDetectColorTerminalCheck(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")

	// Save and override terminal check.
	oldFunc := isTerminalFunc
	isTerminalFunc = func(*os.File) bool { return true }
	defer func() { isTerminalFunc = oldFunc }()

	opts := &Options{ColorAuto: true}
	c := NewColorizer(opts)

	if !c.Enabled() {
		t.Error("color should be enabled when terminal check returns true")
	}
}

func TestDetectColorNotTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")

	oldFunc := isTerminalFunc
	isTerminalFunc = func(*os.File) bool { return false }
	defer func() { isTerminalFunc = oldFunc }()

	opts := &Options{ColorAuto: true}
	c := NewColorizer(opts)

	if c.Enabled() {
		t.Error("color should be disabled when not a terminal")
	}
}

func TestDetectWidthFromTerminal(t *testing.T) {
	t.Setenv("COLUMNS", "")

	oldFunc := getTermSizeFunc
	getTermSizeFunc = func(int) (int, int, error) { return 132, 50, nil }
	defer func() { getTermSizeFunc = oldFunc }()

	opts := &Options{Width: 0}
	w := ResolveWidth(opts)

	if w != 132 {
		t.Errorf("expected 132 from terminal, got %d", w)
	}
}

func TestDetectWidthTerminalError(t *testing.T) {
	t.Setenv("COLUMNS", "")

	oldFunc := getTermSizeFunc
	getTermSizeFunc = func(int) (int, int, error) { return 0, 0, os.ErrNotExist }
	defer func() { getTermSizeFunc = oldFunc }()

	opts := &Options{Width: 0}
	w := ResolveWidth(opts)

	if w != 80 {
		t.Errorf("expected default 80, got %d", w)
	}
}
