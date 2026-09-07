package router

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
)

func errorJSON(c fiber.Ctx, status int, err any) error {
	return c.Status(status).JSON(fiber.Map{"error": fmt.Sprint(err)})
}

func writeJSON(c fiber.Ctx, status int, value any) error {
	return c.Status(status).JSON(value)
}
