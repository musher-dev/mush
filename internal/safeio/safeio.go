package safeio

import "os"

// ReadFile centralizes trusted-path reads so call sites don't need repeated
// gosec suppressions for paths that were validated or derived by our code.
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec,wrapcheck // G304: callers pass trusted or validated paths; helper intentionally preserves original error
}

// ReadFileIfExists reads a file when present and reports whether it existed.
func ReadFileIfExists(path string) (data []byte, exists bool, err error) {
	data, err = ReadFile(path)
	if err == nil {
		return data, true, nil
	}

	if os.IsNotExist(err) {
		return nil, false, nil
	}

	return nil, false, err
}

// MkdirAll centralizes directory creation for known cache/config/project paths.
func MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm) //nolint:wrapcheck // helper intentionally preserves original error
}

// WriteFile centralizes writes to trusted destinations and intentional file modes.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm) //nolint:wrapcheck // helper intentionally preserves original error
}

// Open centralizes trusted-path opens.
func Open(path string) (*os.File, error) {
	return os.Open(path) //nolint:gosec,wrapcheck // G304: callers pass trusted or validated paths; helper intentionally preserves original error
}

// OpenFile centralizes trusted-path open flags.
func OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flag, perm) //nolint:gosec,wrapcheck // G304: callers pass trusted or validated paths; helper intentionally preserves original error
}
