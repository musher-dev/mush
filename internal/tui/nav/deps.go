package nav

import (
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
)

// Dependencies holds external services needed by the TUI.
type Dependencies struct {
	Client  *client.Client // nil if unauthenticated
	Config  *config.Config
	WorkDir string
}

// Action identifies what the TUI wants the caller to do after exit.
type Action int

const (
	// ActionNone means the user quit without selecting an action.
	ActionNone Action = iota
	// ActionBundleLoad means the user completed the bundle loading flow.
	ActionBundleLoad
	// ActionWorkerStart means the user completed the worker entry flow.
	ActionWorkerStart
)

// Result carries the TUI's chosen action and associated parameters back to the caller.
type Result struct {
	Action     Action
	BundleSlug string
	BundleVer  string
	Harness    string
	CachePath  string

	// Worker start fields
	HabitatID          string
	QueueID            string
	QueueName          string
	SupportedHarnesses []string
}
