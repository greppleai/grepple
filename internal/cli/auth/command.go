// Package auth implements authentication and AI-provider command objects.
package auth

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

// Dependencies supplies shared credential and provider operations.
type Dependencies struct {
	AIProvider func([]string) error
	Login      func([]string) error
	Logout     func([]string) error
}

type operation string

const (
	operationAIProvider operation = "ai-provider"
	operationLogin      operation = "login"
	operationLogout     operation = "logout"
)

type command struct {
	dependencies Dependencies
	operation    operation
}

// NewAIProvider constructs the AI-provider command.
func NewAIProvider(dependencies Dependencies) cliruntime.Command {
	return &command{dependencies: dependencies, operation: operationAIProvider}
}

// NewLogin constructs the remote-login command.
func NewLogin(dependencies Dependencies) cliruntime.Command {
	return &command{dependencies: dependencies, operation: operationLogin}
}

// NewLogout constructs the remote-logout command.
func NewLogout(dependencies Dependencies) cliruntime.Command {
	return &command{dependencies: dependencies, operation: operationLogout}
}

// Run executes the configured authentication operation.
func (command *command) Run(args []string) error {
	var execute func([]string) error
	switch command.operation {
	case operationAIProvider:
		execute = command.dependencies.AIProvider
	case operationLogin:
		execute = command.dependencies.Login
	case operationLogout:
		execute = command.dependencies.Logout
	}
	if execute == nil {
		return fmt.Errorf("%s command is unavailable", command.operation)
	}
	return execute(args)
}
