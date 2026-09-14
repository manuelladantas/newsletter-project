package favorites

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func RegisterRoutes(app *fiber.App, store Store) {
	app.Post("/favorites", func(c *fiber.Ctx) error {
		var in Favorite
		if err := c.BodyParser(&in); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
		}
		if strings.TrimSpace(in.URL) == "" || strings.TrimSpace(in.Title) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "url and title are required"})
		}
		in.ID = 0
		saved, created, err := store.Add(c.Context(), in)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if created {
			return c.Status(fiber.StatusCreated).JSON(saved)
		}
		return c.JSON(saved)
	})

	app.Delete("/favorites/:id", func(c *fiber.Ctx) error {
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id must be an integer"})
		}
		if err := store.Delete(c.Context(), id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	// Registered before "/favorites" GET so the literal segment isn't shadowed.
	app.Get("/favorites/urls", func(c *fiber.Ctx) error {
		urls, err := store.URLs(c.Context())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(urls)
	})

	app.Get("/favorites", func(c *fiber.Ctx) error {
		ctx := c.Context()
		total, err := store.Count(ctx)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		totalPages := (total + PageSize - 1) / PageSize

		page, err := strconv.Atoi(c.Query("page"))
		if err != nil || page < 1 {
			page = 1
		}
		if page > totalPages {
			page = max(totalPages, 1)
		}

		items, err := store.List(ctx, (page-1)*PageSize, PageSize)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if items == nil {
			items = []Favorite{}
		}
		return c.JSON(Page{Items: items, Page: page, PageSize: PageSize, TotalItems: total, TotalPages: totalPages})
	})
}
