// Package cliruntime contains the shared command protocol and execution primitives.
package cliruntime

// CommonArgs contains options shared by commands that can contact a Grepple service.
type CommonArgs struct {
	Server string `arg:"-s,--server" placeholder:"URL" help:"remote Grepple service URL"`
}
