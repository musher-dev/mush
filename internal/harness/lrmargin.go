//go:build unix

package harness

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	lrMarginProbeTimeout = 250 * time.Millisecond
	lrMarginDrainTimeout = 30 * time.Millisecond
)

func probeLRMarginSupport(stdin, stdout *os.File, timeout time.Duration, termWidth int) (supported bool, userInput []byte) {
	if stdin == nil || stdout == nil {
		return false, nil
	}

	// Probe reads stdin in non-blocking mode for up to `timeout` (typically 250ms).
	// Any keystrokes typed during this window are preserved: after the probe and
	// drain phases, recognized terminal responses (DECRQM, CPR) are stripped and
	// remaining bytes (user keystrokes) are returned for replay into the executor PTY.

	// Compute probe margins dynamically based on terminal width.
	// Ghostty requires at least 2 columns between margins.
	// ClampTerminalSize guarantees termWidth >= 20.
	probeLeft := max(2, termWidth/4)
	probeRight := max(probeLeft+2, termWidth/2)

	// Save cursor, probe support and behavior, then restore state.
	if _, err := stdout.WriteString("\x1b7"); err != nil {
		return false, nil
	}

	defer func() {
		_, _ = stdout.WriteString("\x1b8\x1b[?69l")
	}()

	probeSeq := fmt.Sprintf("\x1b[?69$p\x1b[?69h\x1b[%d;%ds\x1b[1;%dH\r\x1b[6n", probeLeft, probeRight, probeRight)
	if _, err := stdout.WriteString(probeSeq); err != nil {
		return false, nil
	}

	stdinFd := int(stdin.Fd())

	flags, err := unix.FcntlInt(uintptr(stdinFd), unix.F_GETFL, 0)
	if err != nil {
		return false, nil
	}

	if err := unix.SetNonblock(stdinFd, true); err != nil {
		return false, nil
	}

	defer func() {
		_ = unix.SetNonblock(stdinFd, flags&unix.O_NONBLOCK != 0)
	}()

	deadline := time.Now().Add(timeout)
	accum := make([]byte, 0, 128)
	readBuf := make([]byte, 256)
	modeSupported := false
	modeDecided := false
	result := false

	for time.Now().Before(deadline) {
		n, readErr := stdin.Read(readBuf)
		if n > 0 {
			accum = append(accum, readBuf[:n]...)
			if len(accum) > 512 {
				accum = accum[len(accum)-512:]
			}

			if !modeDecided {
				if modeOK, decided := parseDECRQM69Response(accum); decided {
					modeSupported = modeOK
					modeDecided = true

					if !modeSupported {
						break
					}
				}
			}

			if col, ok := parseCPRColumn(accum); ok {
				// Behavioral check: with margins set, carriage return should
				// land at the left margin, not absolute column 1.
				result = col == probeLeft && (modeSupported || !modeDecided)

				break
			}
		}

		if readErr != nil {
			if errors.Is(readErr, unix.EAGAIN) || errors.Is(readErr, unix.EWOULDBLOCK) {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			break
		}
	}

	// Drain any late-arriving terminal responses so they don't leak into the
	// child process's stdin as garbage (e.g. a delayed DECRQM reply).
	// Drained bytes are accumulated so user keystrokes can be preserved.
	drainDeadline := time.Now().Add(lrMarginDrainTimeout)
	for time.Now().Before(drainDeadline) {
		n, err := stdin.Read(readBuf)
		if n > 0 {
			accum = append(accum, readBuf[:n]...)
			continue
		}

		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				time.Sleep(5 * time.Millisecond)
				continue
			}

			break
		}
	}

	// Strip recognized terminal response sequences; any remaining bytes
	// are user keystrokes that should be replayed into the executor PTY.
	userInput = stripTerminalResponses(accum)
	supported = result

	return supported, userInput
}

func parseDECRQM69Response(data []byte) (supported, decided bool) {
	switch {
	case bytes.Contains(data, []byte("?69;1$y")),
		bytes.Contains(data, []byte("?69;2$y")),
		bytes.Contains(data, []byte("?69;3$y")):
		return true, true
	case bytes.Contains(data, []byte("?69;0$y")),
		bytes.Contains(data, []byte("?69;4$y")):
		return false, true
	default:
		return false, false
	}
}

func parseCPRColumn(data []byte) (col int, ok bool) {
	// Parse the most recent CSI row;colR response.
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] != 'R' {
			continue
		}

		start := -1

		for j := i - 1; j >= 0; j-- {
			if data[j] == 0x1b && j+1 < len(data) && data[j+1] == '[' {
				start = j
				break
			}

			// Bound scan for safety.
			if i-j > 20 {
				break
			}
		}

		if start == -1 || start+2 >= i {
			continue
		}

		payload := string(data[start+2 : i]) // row;col

		parts := strings.Split(payload, ";")
		if len(parts) != 2 {
			continue
		}

		c, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}

		return c, true
	}

	return 0, false
}

// stripTerminalResponses removes recognized terminal response sequences
// (DECRQM mode 69 replies and CPR responses) from data, returning any
// remaining bytes (user keystrokes, arrow keys, etc.) intact.
func stripTerminalResponses(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}

	out := make([]byte, 0, len(data))
	i := 0

	for i < len(data) {
		if data[i] == 0x1b {
			if matched, end := matchDECRQM69(data, i); matched {
				i = end
				continue
			}

			if matched, end := matchCPR(data, i); matched {
				i = end
				continue
			}
		}

		out = append(out, data[i])
		i++
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

// matchDECRQM69 checks if data[start:] begins with a DECRQM mode 69
// response: ESC [ ? 69 ; <digit> $ y
// Returns (true, endIndex) on match, where endIndex is the index after
// the last byte of the sequence.
func matchDECRQM69(data []byte, start int) (matched bool, endIndex int) {
	// Minimum length: ESC [ ? 6 9 ; <d> $ y = 9 bytes
	prefix := []byte("\x1b[?69;")

	if start+9 > len(data) {
		return false, 0
	}

	for j, b := range prefix {
		if data[start+j] != b {
			return false, 0
		}
	}

	d := data[start+6]
	if d < '0' || d > '4' {
		return false, 0
	}

	if data[start+7] != '$' || data[start+8] != 'y' {
		return false, 0
	}

	return true, start + 9
}

// matchCPR checks if data[start:] begins with a CPR (Cursor Position Report)
// response: ESC [ <digits> ; <digits> R
// Returns (true, endIndex) on match.
func matchCPR(data []byte, start int) (matched bool, endIndex int) {
	// Minimum: ESC [ <d> ; <d> R = 6 bytes
	if start+2 >= len(data) || data[start] != 0x1b || data[start+1] != '[' {
		return false, 0
	}

	i := start + 2
	if i >= len(data) || data[i] < '0' || data[i] > '9' {
		return false, 0
	}

	for i < len(data) && data[i] >= '0' && data[i] <= '9' {
		i++
	}

	if i >= len(data) || data[i] != ';' {
		return false, 0
	}

	i++ // skip ';'

	if i >= len(data) || data[i] < '0' || data[i] > '9' {
		return false, 0
	}

	for i < len(data) && data[i] >= '0' && data[i] <= '9' {
		i++
	}

	if i >= len(data) || data[i] != 'R' {
		return false, 0
	}

	return true, i + 1
}

func supportsLRMargins(termName string) bool {
	t := strings.ToLower(strings.TrimSpace(termName))
	if t == "" || t == "dumb" {
		return false
	}

	return true
}
