package cli

import (
	"github.com/greppleai/grepple/internal/authstate"
	authcommand "github.com/greppleai/grepple/internal/cli/auth"
)

func authenticationDependencies() authcommand.Dependencies {
	return authcommand.Dependencies{ServerDefault: serverDefault, StoreLogin: authstate.StoreLogin, ClearToken: authstate.Clear, ConfigPath: authstate.Path}
}
