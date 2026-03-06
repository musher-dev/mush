//go:build !unix

package gemini

import (
	_ "embed"

	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

//go:embed spec.yaml
var specData []byte

var spec = harnesstype.MustParseSpec(specData)

// Module exposes provider metadata on non-unix builds.
var Module = harnesstype.Module{
	Spec:    spec,
	MCPSpec: nil,
}
