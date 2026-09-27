package rules

import (
	"fmt"
	"io"

	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/apiclient"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type commonArgs = cliruntime.CommonArgs

type dependencies struct{ cliruntime.Context }

type command struct{ dependencies dependencies }

// New constructs the rules command.
func New(context cliruntime.Context) cliruntime.Command {
	return &command{dependencies: dependencies{Context: context}}
}

func (dependencies dependencies) serverDefault(value string) string {
	if configuration := dependencies.Configuration(); configuration != nil {
		return configuration.ServerDefault(value)
	}
	return value
}
func (dependencies dependencies) client() apiclient.APIClient { return dependencies.APIClient() }
func (dependencies dependencies) requestExit(code int)        { dependencies.RequestExit(code) }
func readGritQuery(reader io.Reader) (string, error) {
	content, err := io.ReadAll(io.LimitReader(reader, wire.MaxGritQueryBytes+1))
	if err != nil {
		return "", err
	}
	if len(content) > wire.MaxGritQueryBytes {
		return "", fmt.Errorf("structural query exceeds the %d-byte maximum", wire.MaxGritQueryBytes)
	}
	return string(content), nil
}
