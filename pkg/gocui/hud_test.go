package gocui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// deskgit: the HUD frame (Gui.Brackets).

// row reads cells x0..x1 of line y off the screen.
func row(y, x0, x1 int) string {
	var b strings.Builder
	for x := x0; x <= x1; x++ {
		str, _, _ := Screen.Get(x, y)
		b.WriteString(str)
	}
	return b.String()
}

func hudView(t *testing.T, g *Gui, title string) *View {
	t.Helper()
	v, _ := g.SetView("panel", 0, 0, 20, 4, 0)
	v.Title = title
	assert.NoError(t, g.flush())
	return v
}

func TestBracketsMarkTwoCorners(t *testing.T) {
	g := newTestGui(t)
	g.Brackets = true
	g.BracketColor = ColorYellow
	hudView(t, g, "")

	// Top-left: a heavy corner with an arm along each edge.
	assert.Equal(t, "┏━─", row(0, 0, 2))
	assert.Equal(t, "┃", row(1, 0, 0))
	// Bottom-right: the same, turned round.
	assert.Equal(t, "─━┛", row(4, 18, 20))
	assert.Equal(t, "┃", row(3, 20, 20))
	// The other two corners are the frame's own.
	assert.Equal(t, "┐", row(0, 20, 20))
	assert.Equal(t, "└", row(4, 0, 0))

	// The brackets are the bracket colour; the rest of the frame is not.
	_, bracket, _ := Screen.Get(0, 0)
	_, frame, _ := Screen.Get(20, 0)
	assert.NotEqual(t, frame.GetForeground(), bracket.GetForeground(), "a bracket is its own colour, not the frame's")
}

func TestBracketsLeaveAnUnmarkedFrameAlone(t *testing.T) {
	g := newTestGui(t)
	hudView(t, g, "Files")

	assert.Equal(t, "┌─Files─", row(0, 0, 7))
	assert.Equal(t, "┘", row(4, 20, 20))
}

func TestHudHeadIsInCapitalsWithAGapEitherSide(t *testing.T) {
	g := newTestGui(t)
	g.Brackets = true
	v := hudView(t, g, "Files")

	assert.Equal(t, "┏ FILES ─", row(0, 0, 8))
	// Drawn in capitals; the view's own title is what it was set to.
	assert.Equal(t, "Files", v.Title)
}

func TestHudHeadWithANumberAndTabs(t *testing.T) {
	g := newTestGui(t)
	g.Brackets = true
	v, _ := g.SetView("panel", 0, 0, 40, 4, 0)
	v.TitlePrefix = "02"
	v.Tabs = []string{"Files", "Worktrees"}
	assert.NoError(t, g.flush())

	assert.Equal(t, "┏ 02 FILES · WORKTREES ─", row(0, 0, 23))
}

func TestBracketsSkipAViewTooSmallForThem(t *testing.T) {
	g := newTestGui(t)
	g.Brackets = true
	// One line tall: its two corners are the same two cells.
	v, _ := g.SetView("thin", 0, 0, 20, 1, 0)
	_ = v
	assert.NoError(t, g.flush())

	assert.Equal(t, "┌", row(0, 0, 0))
	assert.Equal(t, "┘", row(1, 20, 20))
}

func TestBracketsKeepTheScrollbarAtTheBottomCorner(t *testing.T) {
	g := newTestGui(t)
	g.Brackets = true
	v, _ := g.SetView("panel", 0, 0, 20, 4, 0)
	v.SetContent("1\n2\n3\n4\n5\n6\n7\n8\n9\n")
	// Scrolled to the end: the scrollbar is on the last inner row.
	v.SetOrigin(0, 6)
	assert.NoError(t, g.flush())

	assert.Equal(t, "▐", row(3, 20, 20), "the scrollbar says where you are; the bracket does not get to cover it")
	assert.Equal(t, "━┛", row(4, 19, 20))
}
