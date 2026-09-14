package favorites

import "context"

// PageSize is the fixed number of favorites returned per page by GET /favorites.
const PageSize = 10

// Favorite is a saved snapshot of a digest pick.
type Favorite struct {
	ID     int64  `json:"id"`
	Source string `json:"source"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

// Page is the response shape of GET /favorites.
type Page struct {
	Items      []Favorite `json:"items"`
	Page       int        `json:"page"`
	PageSize   int        `json:"pageSize"`
	TotalItems int        `json:"totalItems"`
	TotalPages int        `json:"totalPages"`
}

type Store interface {
	// Add saves f, or returns the existing favorite with the same URL (created=false).
	Add(ctx context.Context, f Favorite) (saved Favorite, created bool, err error)
	// Delete removes the favorite with the given id; a missing id is not an error.
	Delete(ctx context.Context, id int64) error
	// List returns favorites ordered newest-first (id DESC).
	List(ctx context.Context, offset, limit int) ([]Favorite, error)
	Count(ctx context.Context) (int, error)
	// URLs maps every favorited URL to its id.
	URLs(ctx context.Context) (map[string]int64, error)
}
