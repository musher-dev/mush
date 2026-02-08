//go:build unix

package harness

// Common output keys:
// - success (bool)
// - output (string)          Human-readable output for the platform UI
// - durationMs (int)         Wall-clock duration
// - stdout/stderr (string)   Raw output for debugging (when available)
// - exitCode (int)           Process exit code (when available)
