// Package auth implements authentication and AI-provider command objects.
package auth

import (
	"fmt"
	"io"
	"os"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

// Dependencies supplies shared credential operations.
type Dependencies struct {
	AIProvider    func([]string) error
	Login         func([]string) error
	Logout        func([]string) error
	ServerDefault func(string) string
	StoreLogin    func(string, string, int, int, string) error
	ClearToken    func() error
	ConfigPath    func() (string, error)
	Stderr        io.Writer
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
func (dependencies Dependencies) stderr() io.Writer {
	if dependencies.Stderr != nil {
		return dependencies.Stderr
	}
	return os.Stderr
}
func (dependencies Dependencies) server(value string) string {
	if dependencies.ServerDefault != nil {
		return dependencies.ServerDefault(value)
	}
	return value
}

// Run executes the configured authentication operation.
func (command *command) Run(args []string) error {
	var execute func([]string) error
	switch command.operation {
	case operationAIProvider:
		execute = command.dependencies.AIProvider
		if execute == nil {
			execute = RunAIProvider
		}
	case operationLogin:
		execute = command.dependencies.Login
		if execute == nil {
			execute = func(args []string) error { return executeLogin(args, command.dependencies) }
		}
	case operationLogout:
		execute = command.dependencies.Logout
		if execute == nil {
			execute = func(args []string) error { return executeLogout(args, command.dependencies) }
		}
	}
	if execute == nil {
		return fmt.Errorf("%s command is unavailable", command.operation)
	}
	return execute(args)
}
