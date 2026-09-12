package digest

import (
	"errors"

	"github.com/gofiber/fiber/v2"
)

func RegisterRoutes(app *fiber.App, p *Pipeline, store Store) {
	app.Get("/digest/today", func(c *fiber.Ctx) error {
		d, err := store.Latest(c.Context())
		if errors.Is(err, ErrNoDigest) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no digest yet"})
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(d)
	})

	app.Post("/digest/run", func(c *fiber.Ctx) error {
		d, err := p.Run(c.Context())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(d)
	})
}
