package cli

import anchorscommand "github.com/greppleai/grepple/internal/cli/anchors"

func anchorsDependencies() anchorscommand.Dependencies {
	return anchorscommand.Dependencies{Help: writeAnchorsHelp, Doctor: runAnchorsDoctor, Setup: runAnchorsSetup}
}

func runAnchors(args []string) error { return anchorscommand.New(anchorsDependencies()).Run(args) }
