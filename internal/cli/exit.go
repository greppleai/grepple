package cli

import cliruntime "github.com/greppleai/grepple/internal/cliruntime"

// ExitCode returns a requested non-error command status such as no matches.
func ExitCode(err error) (int, bool) { return cliruntime.ExitCode(err) }
