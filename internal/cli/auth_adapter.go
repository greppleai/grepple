package cli

import authcommand "github.com/greppleai/grepple/internal/cli/auth"

func authenticationDependencies() authcommand.Dependencies {
	return authcommand.Dependencies{AIProvider: runAIProvider, Login: runLogin, Logout: runLogout}
}
