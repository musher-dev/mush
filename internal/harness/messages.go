// Package harness provides a PTY-based TUI for embedding Claude Code.
package harness

// ConnectionStatus represents the connection state.
type ConnectionStatus int

// ConnectionStatus values.
const (
	StatusDisconnected ConnectionStatus = iota
	StatusConnecting
	StatusStarting
	StatusReady
	StatusConnected
	StatusProcessing
	StatusError
)

// String returns a human-readable status.
func (s ConnectionStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "Disconnected"
	case StatusConnecting:
		return "Connecting..."
	case StatusStarting:
		return "Starting..."
	case StatusReady:
		return "Ready"
	case StatusConnected:
		return "Connected"
	case StatusProcessing:
		return "Processing"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}
