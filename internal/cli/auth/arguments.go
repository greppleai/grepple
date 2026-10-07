package auth

import (
	"fmt"
	"net/http"
	"time"

	"github.com/greppleai/grepple/internal/aiprovider"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type ProviderLoginArgs = aiProviderLoginArgs
type ProviderNameArgs = aiProviderNameArgs
type EmptyArgs struct{}

type AIProviderArgs struct {
	Login  *ProviderLoginArgs `arg:"subcommand:login"`
	Logout *ProviderNameArgs  `arg:"subcommand:logout"`
	List   *EmptyArgs         `arg:"subcommand:list"`
}

// LoginArgs selects the local authentication server and browser behavior.
type LoginArgs struct {
	URL       string `arg:"--url" placeholder:"URL" help:"grepple authentication server URL"`
	NoBrowser bool   `arg:"--no-browser" help:"do not attempt to open a browser"`
}

type LogoutArgs struct{}

func ExecuteAIProvider(application cliruntime.Context, values *AIProviderArgs) error {
	if values == nil {
		return fmt.Errorf("ai-provider requires login, logout, or list")
	}
	store, err := aiprovider.NewStore()
	if err != nil {
		return err
	}
	registry := aiprovider.NewRegistry(store, &http.Client{Timeout: 30 * time.Second})
	switch {
	case values.Login != nil:
		return executeAIProviderLogin(application, registry, store, values.Login)
	case values.Logout != nil:
		return executeAIProviderLogout(application, registry, values.Logout)
	case values.List != nil:
		return executeAIProviderList(application, registry)
	default:
		return fmt.Errorf("ai-provider requires login, logout, or list")
	}
}

// ExecuteLogin performs server-owned browser/device authorization.
func ExecuteLogin(application cliruntime.Context, values *LoginArgs) error {
	return executeLoginArgs(application, values)
}

// ExecuteLogout revokes the stored server session and removes local credentials.
func ExecuteLogout(application cliruntime.Context, _ *LogoutArgs) error {
	return executeLogout(application, nil)
}
