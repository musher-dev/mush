//go:build unix

package harness

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	"github.com/musher-dev/mush/internal/harness/harnesstype"
	"github.com/musher-dev/mush/internal/harness/ui/layout"
)

type testInputExecutor struct {
	writes [][]byte
	rows   int
	cols   int
}

func (e *testInputExecutor) Setup(context.Context, *harnesstype.SetupOptions) error { return nil }
func (e *testInputExecutor) Execute(context.Context, *client.Job) (*harnesstype.ExecResult, error) {
	return &harnesstype.ExecResult{}, nil
}
func (e *testInputExecutor) Reset(context.Context) error { return nil }
func (e *testInputExecutor) Teardown()                   {}
func (e *testInputExecutor) Resize(rows, cols int)       { e.rows, e.cols = rows, cols }
func (e *testInputExecutor) WriteInput(p []byte) (int, error) {
	cp := append([]byte(nil), p...)
	e.writes = append(e.writes, cp)

	return len(p), nil
}

func newTestRuntime(t *testing.T) *embeddedRuntime {
	t.Helper()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	t.Cleanup(screen.Fini)

	frame := layout.ComputeFrame(140, 20, true)
	r := &embeddedRuntime{
		ctx:                t.Context(),
		screen:             screen,
		vt:                 vt10x.New(vt10x.WithSize(frame.ViewportWidth, layout.PtyRowsForFrame(&frame))),
		width:              frame.Width,
		height:             frame.Height,
		frame:              frame,
		scrollback:         newScrollbackBuffer(256),
		followTail:         true,
		cfg:                config.Load(),
		supportedHarnesses: []string{"test"},
		executors:          map[string]harnesstype.Executor{},
		sidebarExpanded:    make(map[string]bool),
		now:                time.Now,
		jobs: &JobLoop{
			status:        StatusReady,
			lastHeartbeat: time.Now(),
		},
	}

	return r
}

func seedScrollback(r *embeddedRuntime, count int) {
	for i := 0; i < count; i++ {
		r.scrollback.Push(makeGlyphs(strings.Repeat(string(rune('a'+(i%26))), r.frame.ViewportWidth)))
	}

	r.viewportTop = r.maxViewportTop()
}

func TestHandleMouse_WheelStaysWithinPane(t *testing.T) {
	r := newTestRuntime(t)
	seedScrollback(r, 20)

	initialTop := r.viewportTop
	r.handleMouse(tcell.NewEventMouse(0, 0, tcell.WheelUp, 0))

	if r.viewportTop != initialTop {
		t.Fatalf("top bar wheel changed viewportTop from %d to %d", initialTop, r.viewportTop)
	}

	r.handleMouse(tcell.NewEventMouse(0, layout.TopBarHeight+1, tcell.WheelUp, 0))

	if r.viewportTop != initialTop {
		t.Fatalf("sidebar wheel changed viewportTop from %d to %d", initialTop, r.viewportTop)
	}

	r.handleMouse(tcell.NewEventMouse(r.frame.PaneXStart-1, r.frame.ContentTop, tcell.WheelUp, 0))

	if r.viewportTop >= initialTop {
		t.Fatalf("pane wheel did not move viewport up: before=%d after=%d", initialTop, r.viewportTop)
	}
}

func TestHandleMouse_ForwardsWheelInAltScreen(t *testing.T) {
	r := newTestRuntime(t)
	exec := &testInputExecutor{}
	r.executors = map[string]harnesstype.Executor{"test": exec}
	_, _ = r.vt.Write([]byte("\x1b[?1049h"))
	r.syncMouseCaptureLocked()

	r.handleMouse(tcell.NewEventMouse(r.frame.PaneXStart-1, r.frame.ContentTop, tcell.WheelUp, 0))

	if len(exec.writes) != 1 {
		t.Fatalf("WriteInput calls = %d, want 1", len(exec.writes))
	}

	if got := string(exec.writes[0]); !strings.Contains(got, "\x1b[") {
		t.Fatalf("mouse payload = %q, want CSI sequence", got)
	}

	if !r.mouseCaptureEnabled {
		t.Fatal("mouseCaptureEnabled = false, want true in alt screen")
	}
}

func TestHandleMouse_DoesNotForwardWhenChildDoesNotOwnMouse(t *testing.T) {
	r := newTestRuntime(t)
	exec := &testInputExecutor{}
	r.executors = map[string]harnesstype.Executor{"test": exec}
	r.syncMouseCaptureLocked()

	r.handleMouse(tcell.NewEventMouse(r.frame.PaneXStart-1, r.frame.ContentTop, tcell.WheelUp, 0))

	if len(exec.writes) != 0 {
		t.Fatalf("WriteInput calls = %d, want 0", len(exec.writes))
	}

	if !r.mouseCaptureEnabled {
		t.Fatal("mouseCaptureEnabled = false, want true (runtime always captures mouse for its own interactions)")
	}
}

func TestHandleKey_PageHomeEndUseViewportModel(t *testing.T) {
	r := newTestRuntime(t)
	seedScrollback(r, 30)

	r.handleKey(tcell.NewEventKey(tcell.KeyPgUp, 0, 0))

	if r.followTail {
		t.Fatal("followTail = true after PgUp, want false")
	}

	pagedTop := r.viewportTop
	r.handleKey(tcell.NewEventKey(tcell.KeyHome, 0, 0))

	if r.viewportTop != 0 {
		t.Fatalf("viewportTop = %d after Home, want 0", r.viewportTop)
	}

	r.handleKey(tcell.NewEventKey(tcell.KeyEnd, 0, 0))

	if !r.followTail {
		t.Fatal("followTail = false after End, want true")
	}

	if r.viewportTop <= pagedTop {
		t.Fatalf("viewportTop = %d after End, want > %d", r.viewportTop, pagedTop)
	}
}

func TestHandleResize_InvalidatesHistoryOnWidthChange(t *testing.T) {
	r := newTestRuntime(t)
	exec := &testInputExecutor{}
	r.executors = map[string]harnesstype.Executor{"test": exec}
	seedScrollback(r, 12)
	r.followTail = false
	r.viewportTop = 3

	r.handleResize(120, 20)

	if r.scrollback.Len() != 0 {
		t.Fatalf("scrollback Len() = %d, want 0", r.scrollback.Len())
	}

	if r.historyNotice == "" {
		t.Fatal("historyNotice is empty after width change")
	}

	if !r.followTail || r.viewportTop != 0 {
		t.Fatalf("followTail=%v viewportTop=%d, want true/0", r.followTail, r.viewportTop)
	}

	if exec.cols != r.frame.ViewportWidth {
		t.Fatalf("Resize cols = %d, want %d", exec.cols, r.frame.ViewportWidth)
	}
}

func TestGlyphRowsEqual_DetectsStyleOnlyChanges(t *testing.T) {
	left := [][]vt10x.Glyph{{{Char: 'x', FG: 1, BG: 2, Mode: 0}}}
	right := [][]vt10x.Glyph{{{Char: 'x', FG: 3, BG: 2, Mode: 0}}}

	if glyphRowsEqual(left, right, 1) {
		t.Fatal("glyphRowsEqual() = true, want false for style-only change")
	}
}

func TestHandleMouse_ScrollbarTrackClickMovesViewport(t *testing.T) {
	r := newTestRuntime(t)
	seedScrollback(r, 40)
	r.followTail = false
	r.viewportTop = 15

	thumbTop, _ := r.scrollbarThumb()
	if thumbTop == 0 {
		t.Fatalf("thumbTop = %d, want > 0 for mid-history viewport", thumbTop)
	}

	y := r.frame.ContentTop - 1 + thumbTop - 1
	x := r.frame.ScrollbarXStart - 1
	before := r.viewportTop

	r.handleMouse(tcell.NewEventMouse(x, y, tcell.Button1, 0))

	if r.viewportTop >= before {
		t.Fatalf("scrollbar track click did not page up: before=%d after=%d", before, r.viewportTop)
	}
}

func TestRenderViewport_AppliesSoftwareCursorWhenCursorVisible(t *testing.T) {
	r := newTestRuntime(t)
	_, _ = r.vt.Write([]byte("A\r")) // cursor visible (default)

	r.renderViewport()

	x := r.frame.PaneXStart - 1
	y := r.frame.ContentTop - 1

	content, style, _ := r.screen.Get(x, y)
	if content != "A" {
		t.Fatalf("content = %q, want A", content)
	}

	fg, bg, _ := style.Decompose()
	if fg != tnPTYBg || bg != tnText {
		t.Fatalf("software cursor colors = (%v,%v), want (%v,%v)", fg, bg, tnPTYBg, tnText)
	}
}

func TestRenderViewport_SoftwareCursorOnlyWhenChildOwnsMouse(t *testing.T) {
	r := newTestRuntime(t)
	// Enable mouse tracking + SGR format without alt-screen, simulating Ink/Claude Code.
	_, _ = r.vt.Write([]byte("prompt>\r\x1b[?1003h\x1b[?1006h"))

	r.renderViewport()

	x := r.frame.PaneXStart - 1
	y := r.frame.ContentTop - 1

	content, style, _ := r.screen.Get(x, y)
	if content != "p" {
		t.Fatalf("content = %q, want p", content)
	}

	// Software cursor IS applied (reversed colors) — Ink sets CursorVisible
	// only after repositioning to the input element.
	fg, bg, _ := style.Decompose()
	if fg != tnPTYBg || bg != tnText {
		t.Fatalf("software cursor colors = (%v,%v), want (%v,%v)", fg, bg, tnPTYBg, tnText)
	}
}

func TestRenderViewport_SuppressesCursorInAltScreen(t *testing.T) {
	r := newTestRuntime(t)
	_, _ = r.vt.Write([]byte("content\r\x1b[?1049h"))

	r.renderViewport()

	x := r.frame.PaneXStart - 1
	y := r.frame.ContentTop - 1

	_, style, _ := r.screen.Get(x, y)

	fg, bg, _ := style.Decompose()
	if fg == tnPTYBg && bg == tnText {
		t.Fatal("cell has software cursor, want no cursor in alt-screen mode")
	}
}

func TestRenderViewport_HidesCursorWhenLiveCursorIsOffscreen(t *testing.T) {
	r := newTestRuntime(t)
	seedScrollback(r, 40)
	r.followTail = false
	r.viewportTop = 0
	_, _ = r.vt.Write([]byte("A\r\x1b[?25l"))

	r.renderViewport()

	x := r.frame.PaneXStart - 1
	y := r.frame.ContentTop - 1
	_, style, _ := r.screen.Get(x, y)

	fg, bg, _ := style.Decompose()
	if fg == tnPTYBg && bg == tnText {
		t.Fatal("top cell looks software-cursor-highlighted, want no software cursor when live cursor is offscreen")
	}
}
