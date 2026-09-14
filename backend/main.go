package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	_ "github.com/lib/pq"

	"newsletter-backend/digest"
	"newsletter-backend/favorites"
)

func main() {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open db connection: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := digest.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	store, err := digest.NewPostgresStore(ctx, db)
	if err != nil {
		log.Fatal(err)
	}
	favStore, err := favorites.NewPostgresStore(ctx, db)
	if err != nil {
		log.Fatal(err)
	}
	pipeline := &digest.Pipeline{
		Sources:         digest.DefaultSources(),
		OllamaURL:       cfg.OllamaURL,
		InterestProfile: cfg.InterestProfile,
		HTTPClient:      &http.Client{Timeout: 2 * time.Minute},
		Store:           store,
		Logger:          log.Default(),
	}
	digest.StartScheduler(ctx, pipeline)

	app := fiber.New()
	app.Use(cors.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		if err := db.Ping(); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString(fmt.Sprintf("db unreachable: %v", err))
		}
		return c.SendString("ok")
	})
	digest.RegisterRoutes(app, pipeline, store)
	favorites.RegisterRoutes(app, favStore)

	addr := ":8090"
	log.Printf("backend listening on %s", addr)
	log.Fatal(app.Listen(addr))
}
