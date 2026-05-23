package api

import "github.com/gofiber/fiber/v2"

// starts with capital letter - public = exported(usable by other packages)
func HealthHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "ok",
		"service": "goflow",
	})
}
