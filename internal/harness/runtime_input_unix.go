//go:build unix

package harness

import (
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"

	"github.com/musher-dev/mush/internal/harness/harnesstype"
	"github.com/musher-dev/mush/internal/harness/ui/layout"
)

func (r *embeddedRuntime) handleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyCtrlQ:
		r.signalDone()

		return true
	case tcell.KeyCtrlC:
		if r.handleCtrlC() {
			return true
		}

		return false
	}

	if !r.isAltScreenActive() {
		switch ev.Key() {
		case tcell.KeyPgUp:
			r.scrollUp(max(layout.PtyRowsForFrame(&r.frame)-1, 1))
			return false
		case tcell.KeyPgDn:
			r.scrollDown(max(layout.PtyRowsForFrame(&r.frame)-1, 1))
			return false
		case tcell.KeyHome:
			r.scrollToTop()
			return false
		case tcell.KeyEnd, tcell.KeyEscape:
			r.scrollToBottom()
			return false
		}
	}

	keyBytes := encodeTCellKey(ev)
	if len(keyBytes) == 0 {
		return false
	}

	if !r.followTail && !r.isAltScreenActive() {
		r.uiMu.Lock()
		r.endScrollLocked()
		r.drawLocked()
		r.uiMu.Unlock()
	}

	r.writeInput(keyBytes)

	return false
}

func encodeTCellKey(ev *tcell.EventKey) []byte {
	switch ev.Key() {
	case tcell.KeyRune:
		ch := ev.Rune()
		buf := make([]byte, utf8.RuneLen(ch))
		utf8.EncodeRune(buf, ch)

		if ev.Modifiers()&tcell.ModAlt != 0 {
			return append([]byte{0x1b}, buf...)
		}

		return buf
	case tcell.KeyEnter:
		return []byte{'\r'}
	case tcell.KeyTab:
		return []byte{'\t'}
	case tcell.KeyBacktab:
		return []byte("\x1b[Z")
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		return []byte{0x7f}
	case tcell.KeyEsc:
		return []byte{0x1b}
	case tcell.KeyUp:
		return []byte("\x1b[A")
	case tcell.KeyDown:
		return []byte("\x1b[B")
	case tcell.KeyRight:
		return []byte("\x1b[C")
	case tcell.KeyLeft:
		return []byte("\x1b[D")
	case tcell.KeyHome:
		return []byte("\x1b[H")
	case tcell.KeyEnd:
		return []byte("\x1b[F")
	case tcell.KeyPgUp:
		return []byte("\x1b[5~")
	case tcell.KeyPgDn:
		return []byte("\x1b[6~")
	case tcell.KeyDelete:
		return []byte("\x1b[3~")
	case tcell.KeyInsert:
		return []byte("\x1b[2~")
	case tcell.KeyCtrlA:
		return []byte{0x01}
	case tcell.KeyCtrlB:
		return []byte{0x02}
	case tcell.KeyCtrlD:
		return []byte{0x04}
	case tcell.KeyCtrlE:
		return []byte{0x05}
	case tcell.KeyCtrlF:
		return []byte{0x06}
	case tcell.KeyCtrlH:
		return []byte{0x08}
	case tcell.KeyCtrlI:
		return []byte{0x09}
	case tcell.KeyCtrlJ:
		return []byte{0x0a}
	case tcell.KeyCtrlK:
		return []byte{0x0b}
	case tcell.KeyCtrlL:
		return []byte{0x0c}
	case tcell.KeyCtrlM:
		return []byte{0x0d}
	case tcell.KeyCtrlN:
		return []byte{0x0e}
	case tcell.KeyCtrlO:
		return []byte{0x0f}
	case tcell.KeyCtrlP:
		return []byte{0x10}
	case tcell.KeyCtrlR:
		return []byte{0x12}
	case tcell.KeyCtrlT:
		return []byte{0x14}
	case tcell.KeyCtrlU:
		return []byte{0x15}
	case tcell.KeyCtrlV:
		return []byte{0x16}
	case tcell.KeyCtrlW:
		return []byte{0x17}
	case tcell.KeyCtrlX:
		return []byte{0x18}
	case tcell.KeyCtrlY:
		return []byte{0x19}
	case tcell.KeyCtrlZ:
		return []byte{0x1a}
	}

	return nil
}

func (r *embeddedRuntime) handleCtrlC() bool {
	if !r.jobs.HasActiveInterruptableJob() {
		r.signalDone()

		return true
	}

	nowFn := r.now
	if nowFn == nil {
		nowFn = time.Now
	}

	now := nowFn()
	secondPress := !r.lastCtrlCAt.IsZero() && now.Sub(r.lastCtrlCAt) <= r.ctrlCExitWindow

	if secondPress {
		r.lastCtrlCAt = time.Time{}
		r.infof("Second Ctrl+C received: exiting watch mode.")
		r.signalDone()

		return true
	}

	r.lastCtrlCAt = now

	if executor, ok := r.executors[r.jobs.CurrentJobHarnessType()]; ok {
		if ih, ok := executor.(harnesstype.InterruptHandler); ok {
			_ = ih.Interrupt()
		}
	}

	r.infof("Interrupt sent to agent. Press Ctrl+C again within %s to exit watch mode.", r.ctrlCExitWindow.Round(time.Second))

	return false
}

func (r *embeddedRuntime) writeInput(keyBytes []byte) {
	for _, harnessType := range r.supportedHarnesses {
		if executor, ok := r.executors[harnessType]; ok {
			if ir, ok := executor.(harnesstype.InputReceiver); ok {
				_, _ = ir.WriteInput(keyBytes)

				return
			}
		}
	}
}

func (r *embeddedRuntime) syncMouseCaptureLocked() {
	if r.screen == nil {
		return
	}

	shouldCapture := r.childOwnsMouse()
	if shouldCapture == r.mouseCaptureEnabled {
		return
	}

	if shouldCapture {
		r.screen.EnableMouse(tcell.MouseButtonEvents | tcell.MouseDragEvents)
	} else {
		r.screen.DisableMouse()
	}

	r.mouseCaptureEnabled = shouldCapture
}
