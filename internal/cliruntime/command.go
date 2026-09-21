package runtime

// Command executes one CLI command from its command-specific arguments.
type Command interface {
	Run(args []string) error
}

// CommandFunc adapts a function to Command during incremental command migrations.
type CommandFunc func(args []string) error

// Run executes command using args.
func (command CommandFunc) Run(args []string) error { return command(args) }
