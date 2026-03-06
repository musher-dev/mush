package nav

import (
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
)

// BundleSeed provides pre-resolved bundle data so the TUI can start
// directly at the bundle action screen (Run/Install choice).
type BundleSeed struct {
	Namespace string
	Slug      string
	Version   string
	CachePath string
}

// Dependencies holds external services needed by the TUI.
type Dependencies struct {
	Client        *client.Client // nil if unauthenticated
	Config        *config.Config
	WorkDir       string
	InitialBundle *BundleSeed // nil = start at home screen
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
	// ActionHarnessInstall means the user wants to install missing harnesses.
	ActionHarnessInstall
	// ActionBareRun means the user wants to run a harness without a bundle.
	ActionBareRun
	// ActionBundleInstall means the user wants to install bundle assets into the working directory.
	ActionBundleInstall
)

// Result carries the TUI's chosen action and associated parameters back to the caller.
type Result struct {
	Action          Action
	BundleNamespace string
	BundleSlug      string
	BundleVer       string
	Harness         string
	CachePath       string

	// Bundle install fields
	Force bool

	// Worker start fields
	HabitatID          string
	QueueID            string
	QueueName          string
	SupportedHarnesses []string

	// Harness install fields
	InstallCommands [][]string
}
