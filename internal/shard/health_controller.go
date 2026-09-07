package shard

import "github.com/gofiber/fiber/v3"

type healthController struct {
	service HealthService
}

func (h *healthController) register(app *fiber.App) {
	app.Get("/", h.ready)
	app.Get("/health", h.health)
	app.Get("/ready", h.ready)
}

func (h *healthController) health(c fiber.Ctx) error {
	return writeJSON(c, fiber.StatusOK, fiber.Map{
		"ok":   true,
		"mode": "shard",
		"root": h.service.Root(),
	})
}

func (h *healthController) ready(c fiber.Ctx) error {
	zoektRunning := h.service.ZoektRunning()
	status := fiber.StatusOK
	if !zoektRunning {
		status = fiber.StatusServiceUnavailable
	}
	return writeJSON(c, status, fiber.Map{
		"ok":    zoektRunning,
		"mode":  "shard",
		"root":  h.service.Root(),
		"zoekt": zoektRunning,
	})
}
