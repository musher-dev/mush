//go:build !unix

package main

import (
	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/tui/nav"
)

func handleBundleLoadNavResult(_ *cobra.Command, _ *output.Writer, _ *nav.Result) error {
	return unsupportedWatchModeError()
}

func handleBareRunNavResult(_ *cobra.Command, _ *output.Writer, _ *nav.Result) error {
	return unsupportedWatchModeError()
}
