// Package auth implements authentication and AI-provider command objects.
package auth

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type operation string

const (
	operationAIProvider operation = "ai-provider"
	operationLogin      operation = "login"
	operationLogout     operation = "logout"
)

type command struct {
	application cliruntime.Context
	operation   operation
	execute     func([]string) error
}

// NewAIProvider constructs the AI-provider command from the common command context.
func NewAIProvider(application cliruntime.Context) cliruntime.Command {
	return newCommand(application, operationAIProvider, nil)
}

// NewLogin constructs the remote-login command from the common command context.
func NewLogin(application cliruntime.Context) cliruntime.Command {
	return newCommand(application, operationLogin, nil)
}

// NewLogout constructs the remote-logout command from the common command context.
func NewLogout(application cliruntime.Context) cliruntime.Command {
	return newCommand(application, operationLogout, nil)
}

func newCommand(application cliruntime.Context, operation operation, execute func([]string) error) cliruntime.Command {
	return &command{application: application, operation: operation, execute: execute}
}

// Run executes the configured authentication operation.
func (command *command) Run(args []string) error {
	if command.application == nil {
		return fmt.Errorf("%s command context is unavailable", command.operation)
	}
	execute := command.execute
	switch command.operation {
	case operationAIProvider:
		if execute == nil {
			execute = func(args []string) error { return runAIProvider(command.application, args) }
		}
	case operationLogin:
		if execute == nil {
			execute = func(args []string) error { return executeLogin(command.application, args) }
		}
	case operationLogout:
		if execute == nil {
			execute = func(args []string) error { return executeLogout(command.application, args) }
		}
	}
	if execute == nil {
		return fmt.Errorf("%s command is unavailable", command.operation)
	}
	return execute(args)
}
