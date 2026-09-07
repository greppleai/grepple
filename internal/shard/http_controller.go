package shard

import (
	"fmt"
	"grepple/internal/api"

	"github.com/gofiber/fiber/v3"
)

// HealthService supplies the liveness data exposed by the health controller.
type HealthService interface {
	Root() string
	ZoektRunning() bool
}

// RuleService supplies rule state and mutations without making the HTTP package
// depend on the shard package.
type RuleService interface {
	ApplyRules(api.RuleSet) bool
	RulesGeneration() int64
	RulesSnapshot() []api.Rule
	RuleResults(string) (string, []api.RuleRepoResult)
}

// Service is the transport-facing surface of the shard domain.
type Service interface {
	Search(api.SearchRequest, string) (api.SearchResponse, error)
	ListRepos() []api.RepoInfo
	GetRepo(string) (api.RepoInfo, bool)
	CreateRepo(api.RepoRef, string) (api.RepoInfo, error)
	RefreshRepo(api.RepoRef, string) (api.RepoInfo, error)
	RemoveRepo(string) (bool, error)
}

// StatusError lets the domain adapter preserve endpoint status semantics while
// returning ordinary errors through the transport interface.
type StatusError struct {
	Status int
	Err    error
}

func (e StatusError) Error() string { return e.Err.Error() }

func statusFor(err error, fallback int) int {
	if statusErr, ok := err.(StatusError); ok {
		return statusErr.Status
	}
	return fallback
}

// Register installs all shard routes. Controllers stay private implementation
// details while their dependencies remain explicit and cycle-free.
func Register(app *fiber.App, health HealthService, shard Service, rules RuleService) {
	(&healthController{service: health}).register(app)
	(&shardController{state: shard}).register(app)
	(&ruleController{rules: rules}).register(app)
}

func errorJSON(c fiber.Ctx, status int, err any) error {
	return c.Status(status).JSON(fiber.Map{"error": fmtSprint(err)})
}

// Kept tiny to avoid formatting differences in the established JSON error API.
func fmtSprint(value any) string {
	if err, ok := value.(error); ok {
		return err.Error()
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func writeJSON(c fiber.Ctx, status int, value any) error {
	return c.Status(status).JSON(value)
}
