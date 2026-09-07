package shard

import (
	"encoding/json"
	"grepple/internal/api"

	"github.com/gofiber/fiber/v3"
)

type ruleController struct {
	rules RuleService
}

func (r *ruleController) register(app *fiber.App) {
	app.Put("/rules", r.handleRulesPut)
	app.Get("/rules", r.handleRulesList)
	app.Get("/rules/:id/results", r.handleRuleResults)
}

func (r *ruleController) handleRulesPut(c fiber.Ctx) error {
	if r.rules == nil {
		return errorJSON(c, fiber.StatusServiceUnavailable, "rules not initialized")
	}
	var set api.RuleSet
	if err := json.Unmarshal(c.Body(), &set); err != nil {
		return errorJSON(c, fiber.StatusBadRequest, "invalid JSON body")
	}
	applied := r.rules.ApplyRules(set)
	return writeJSON(c, fiber.StatusOK, fiber.Map{"applied": applied, "generation": r.rules.RulesGeneration()})
}

func (r *ruleController) handleRulesList(c fiber.Ctx) error {
	if r.rules == nil {
		return writeJSON(c, fiber.StatusOK, api.RuleSet{})
	}
	return writeJSON(c, fiber.StatusOK, api.RuleSet{Generation: r.rules.RulesGeneration(), Rules: r.rules.RulesSnapshot()})
}

func (r *ruleController) handleRuleResults(c fiber.Ctx) error {
	id := c.Params("id")
	if r.rules == nil {
		return writeJSON(c, fiber.StatusOK, api.RuleResults{Rule: id, Repos: []api.RuleRepoResult{}})
	}
	mode, repos := r.rules.RuleResults(id)
	return writeJSON(c, fiber.StatusOK, api.RuleResults{Rule: id, Mode: mode, Generation: r.rules.RulesGeneration(), Repos: repos})
}
