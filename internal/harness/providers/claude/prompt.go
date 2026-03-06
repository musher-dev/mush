//go:build unix

package claude

import "time"

// SignalFileName is the marker file created by the Stop hook.
const SignalFileName = "complete"

// PromptDetectionBytes contains the bytes to detect Claude's input prompt.
// We look for "❯ " (U+276F HEAVY RIGHT-POINTING ANGLE QUOTATION MARK ORNAMENT + space)
// to know Claude is ready for input (used for initial ready state).
var PromptDetectionBytes = []byte{0xe2, 0x9d, 0xaf, 0x20} // "❯ " in UTF-8

// PromptDebounceTime is how long to wait after seeing the prompt before
// declaring Claude is ready. Used only for initial startup detection.
const PromptDebounceTime = 1 * time.Second

// SignalPollInterval is how often to check for completion signal files.
const SignalPollInterval = 200 * time.Millisecond
