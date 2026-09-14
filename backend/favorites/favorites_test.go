package favorites

// Tests for the Favorites feature.
// Every test traces back to specs/favorites/spec.md (FR-n = Functional
// Requirement n, "Edge:" = Edge Cases table row, "AC:" = Acceptance Criteria item,
// "NFR:" = Non-Functional Requirement).
//
// These tests are written before the implementation exists (red step of TDD) and
// are expected to fail to compile until the favorites package is implemented.
//
// Assumed package contract (mirrors backend/digest):
//
//	type Favorite struct { ID int64; Source, Title, URL, Reason string } // json: id, source, title, url, reason
//	type Store interface {
//	    Add(ctx, f Favorite) (saved Favorite, created bool, err error) // idempotent on URL
//	    Delete(ctx, id int64) error                                    // nil when id does not exist
//	    List(ctx, offset, limit int) ([]Favorite, error)               // ordered by id DESC
//	    Count(ctx) (int, error)
//	    URLs(ctx) (map[string]int64, error)
//	}
//	const PageSize = 10
//	func RegisterRoutes(app *fiber.App, store Store)
//	func NewPostgresStore(ctx, db *sql.DB) (*PostgresStore, error)
//
// Tests suffixed "_Integration" need a real Postgres and are skipped unless
// TEST_DATABASE_URL is set (e.g. postgres://user:pass@localhost:5432/db?sslmode=disable).

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	_ "github.com/lib/pq"

	"newsletter-backend/digest"
)

// ---------------------------------------------------------------------------
// Test helpers (test-only; no production logic lives here)
// ---------------------------------------------------------------------------

// fakeStore is an in-memory Store keyed by URL, assigning incrementing ids.
type fakeStore struct {
	mu     sync.Mutex
	nextID int64
	byID   map[int64]Favorite
	err    error // when set, every method returns this error
}

func newFakeStore() *fakeStore {
	return &fakeStore{byID: map[int64]Favorite{}}
}

func (s *fakeStore) Add(_ context.Context, f Favorite) (Favorite, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return Favorite{}, false, s.err
	}
	for _, existing := range s.byID {
		if existing.URL == f.URL {
			return existing, false, nil
		}
	}
	s.nextID++
	f.ID = s.nextID
	s.byID[f.ID] = f
	return f, true, nil
}

func (s *fakeStore) Delete(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	delete(s.byID, id)
	return nil
}

func (s *fakeStore) sorted() []Favorite {
	out := make([]Favorite, 0, len(s.byID))
	for _, f := range s.byID {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *fakeStore) List(_ context.Context, offset, limit int) ([]Favorite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	all := s.sorted()
	if offset >= len(all) {
		return []Favorite{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}

func (s *fakeStore) Count(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0, s.err
	}
	return len(s.byID), nil
}

func (s *fakeStore) URLs(_ context.Context) (map[string]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	m := map[string]int64{}
	for id, f := range s.byID {
		m[f.URL] = id
	}
	return m, nil
}

func (s *fakeStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byID)
}

func newApp(store Store) *fiber.App {
	app := fiber.New()
	RegisterRoutes(app, store)
	return app
}

func samplePick(n int) map[string]string {
	return map[string]string{
		"source": "Hacker News",
		"title":  fmt.Sprintf("Article %d", n),
		"url":    fmt.Sprintf("https://example.com/article-%d", n),
		"reason": fmt.Sprintf("Reason %d", n),
	}
}

func postJSON(t *testing.T, app *fiber.App, path string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if s, ok := body.(string); ok {
		buf.WriteString(s)
	} else if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("encode body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	return resp
}

func do(t *testing.T, app *fiber.App, method, path string) *http.Response {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(method, path, nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var v T
	raw, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode %T from %q: %v", v, raw, err)
	}
	return v
}

type favoriteJSON struct {
	ID     int64  `json:"id"`
	Source string `json:"source"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

type pageJSON struct {
	Items      []favoriteJSON `json:"items"`
	Page       int            `json:"page"`
	PageSize   int            `json:"pageSize"`
	TotalItems int            `json:"totalItems"`
	TotalPages int            `json:"totalPages"`
}

type errorJSON struct {
	Error string `json:"error"`
}

// seed adds n favorites through the API and returns their ids in creation order.
func seed(t *testing.T, app *fiber.App, n int) []int64 {
	t.Helper()
	ids := make([]int64, 0, n)
	for i := 1; i <= n; i++ {
		resp := postJSON(t, app, "/favorites", samplePick(i))
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("seed %d: status %d", i, resp.StatusCode)
		}
		ids = append(ids, decode[favoriteJSON](t, resp).ID)
	}
	return ids
}

// ---------------------------------------------------------------------------
// FR-2: POST /favorites creates a favorite
// ---------------------------------------------------------------------------

// Spec: FR-2, AC "creates a favorite and returns it with an id (201)"
func TestCreateEndpoint_NewFavoriteReturns201WithID(t *testing.T) {
	store := newFakeStore()
	app := newApp(store)

	resp := postJSON(t, app, "/favorites", samplePick(1))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	got := decode[favoriteJSON](t, resp)
	if got.ID == 0 {
		t.Errorf("id = 0, want backend-generated id")
	}
	want := samplePick(1)
	if got.Source != want["source"] || got.Title != want["title"] || got.URL != want["url"] || got.Reason != want["reason"] {
		t.Errorf("returned favorite = %+v, want snapshot of %v", got, want)
	}
	if store.count() != 1 {
		t.Errorf("stored rows = %d, want 1", store.count())
	}
}

// Spec: FR-2, Edge "Same article favorited twice", AC "already-favorited url returns
// the existing row (200) and creates no duplicate", NFR idempotency (POST)
func TestCreateEndpoint_ExistingURLReturns200SameRowNoDuplicate(t *testing.T) {
	store := newFakeStore()
	app := newApp(store)

	first := decode[favoriteJSON](t, postJSON(t, app, "/favorites", samplePick(1)))

	// Second add with the same url (title/reason may differ in a later digest).
	again := samplePick(1)
	again["title"] = "Same article, retitled"
	resp := postJSON(t, app, "/favorites", again)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 for existing url", resp.StatusCode)
	}
	second := decode[favoriteJSON](t, resp)
	if second.ID != first.ID {
		t.Errorf("id = %d, want existing id %d", second.ID, first.ID)
	}
	if second.Title != first.Title {
		t.Errorf("title = %q, want original snapshot %q", second.Title, first.Title)
	}
	if store.count() != 1 {
		t.Errorf("stored rows = %d, want 1 (no duplicate)", store.count())
	}
}

// ---------------------------------------------------------------------------
// FR-3: POST /favorites validation
// ---------------------------------------------------------------------------

// Spec: FR-3, Edge "Add request with missing/blank url or title", AC "returns 400 and stores nothing"
func TestCreateEndpoint_MissingOrBlankFieldsReturn400(t *testing.T) {
	cases := []struct {
		name string
		body map[string]string
	}{
		{"missing url", map[string]string{"source": "Medium", "title": "T", "reason": "r"}},
		{"blank url", map[string]string{"source": "Medium", "title": "T", "url": "", "reason": "r"}},
		{"whitespace url", map[string]string{"source": "Medium", "title": "T", "url": "   ", "reason": "r"}},
		{"missing title", map[string]string{"source": "Medium", "url": "https://x.test/a", "reason": "r"}},
		{"blank title", map[string]string{"source": "Medium", "title": "", "url": "https://x.test/a", "reason": "r"}},
		{"whitespace title", map[string]string{"source": "Medium", "title": " \t", "url": "https://x.test/a", "reason": "r"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			app := newApp(store)

			resp := postJSON(t, app, "/favorites", tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			if e := decode[errorJSON](t, resp); e.Error == "" {
				t.Errorf(`body missing non-empty "error" field`)
			}
			if store.count() != 0 {
				t.Errorf("stored rows = %d, want 0", store.count())
			}
		})
	}
}

// Spec: FR-3 (a body that cannot be parsed has no url/title at all)
func TestCreateEndpoint_MalformedJSONReturns400(t *testing.T) {
	store := newFakeStore()
	app := newApp(store)

	resp := postJSON(t, app, "/favorites", `{"title": "oops"`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if store.count() != 0 {
		t.Errorf("stored rows = %d, want 0", store.count())
	}
}

// ---------------------------------------------------------------------------
// FR-4: DELETE /favorites/:id
// ---------------------------------------------------------------------------

// Spec: FR-4, AC "removes the favorite and returns 204"
func TestDeleteEndpoint_RemovesAndReturns204(t *testing.T) {
	store := newFakeStore()
	app := newApp(store)
	ids := seed(t, app, 2)

	resp := do(t, app, http.MethodDelete, fmt.Sprintf("/favorites/%d", ids[0]))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if store.count() != 1 {
		t.Errorf("stored rows = %d, want 1", store.count())
	}
	urls := decode[map[string]int64](t, do(t, app, http.MethodGet, "/favorites/urls"))
	if _, still := urls[samplePick(1)["url"]]; still {
		t.Errorf("deleted favorite still present in /favorites/urls")
	}
}

// Spec: FR-4, Edge "Unstar an article that's already gone", AC "a second delete of the
// same id also returns 204", NFR idempotency (DELETE)
func TestDeleteEndpoint_UnknownOrRepeatedIDStillReturns204(t *testing.T) {
	store := newFakeStore()
	app := newApp(store)
	ids := seed(t, app, 1)

	if resp := do(t, app, http.MethodDelete, fmt.Sprintf("/favorites/%d", ids[0])); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first delete status = %d, want 204", resp.StatusCode)
	}
	if resp := do(t, app, http.MethodDelete, fmt.Sprintf("/favorites/%d", ids[0])); resp.StatusCode != http.StatusNoContent {
		t.Errorf("second delete status = %d, want 204", resp.StatusCode)
	}
	if resp := do(t, app, http.MethodDelete, "/favorites/999999"); resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete of never-existing id status = %d, want 204", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// FR-5: GET /favorites?page=N
// ---------------------------------------------------------------------------

// Spec: FR-5, AC "returns 10 items per page, newest first, with page, pageSize, totalItems, totalPages"
func TestListEndpoint_ReturnsTenPerPageNewestFirstWithMeta(t *testing.T) {
	store := newFakeStore()
	app := newApp(store)
	ids := seed(t, app, 23)

	page1 := decode[pageJSON](t, do(t, app, http.MethodGet, "/favorites?page=1"))
	if page1.Page != 1 || page1.PageSize != 10 || page1.TotalItems != 23 || page1.TotalPages != 3 {
		t.Errorf("page1 meta = {page:%d pageSize:%d totalItems:%d totalPages:%d}, want {1 10 23 3}",
			page1.Page, page1.PageSize, page1.TotalItems, page1.TotalPages)
	}
	if len(page1.Items) != 10 {
		t.Fatalf("page1 items = %d, want 10", len(page1.Items))
	}
	// Newest saved first: ids descending.
	for i := range page1.Items {
		if want := ids[len(ids)-1-i]; page1.Items[i].ID != want {
			t.Errorf("page1 item %d id = %d, want %d (newest first)", i, page1.Items[i].ID, want)
		}
	}
	if page1.Items[0].Title != samplePick(23)["title"] || page1.Items[0].Source == "" || page1.Items[0].URL == "" || page1.Items[0].Reason == "" {
		t.Errorf("items must carry the full snapshot; got %+v", page1.Items[0])
	}

	page3 := decode[pageJSON](t, do(t, app, http.MethodGet, "/favorites?page=3"))
	if page3.Page != 3 || len(page3.Items) != 3 {
		t.Errorf("page3: page=%d items=%d, want page=3 items=3", page3.Page, len(page3.Items))
	}
	if page3.Items[2].ID != ids[0] {
		t.Errorf("last item on last page id = %d, want oldest id %d", page3.Items[2].ID, ids[0])
	}
}

// ---------------------------------------------------------------------------
// FR-6: page clamping
// ---------------------------------------------------------------------------

// Spec: FR-6, Edge "?page=99 beyond the last page, or ?page=0 / non-numeric",
// AC "clamps missing/non-numeric/<1 page to 1 and >totalPages to the last page"
func TestListEndpoint_ClampsPage(t *testing.T) {
	store := newFakeStore()
	app := newApp(store)
	seed(t, app, 12) // 2 pages

	cases := []struct {
		name     string
		query    string
		wantPage int
		wantLen  int
	}{
		{"missing page defaults to 1", "", 1, 10},
		{"non-numeric clamps to 1", "?page=abc", 1, 10},
		{"zero clamps to 1", "?page=0", 1, 10},
		{"negative clamps to 1", "?page=-3", 1, 10},
		{"beyond last clamps to last", "?page=99", 2, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := do(t, app, http.MethodGet, "/favorites"+tc.query)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			got := decode[pageJSON](t, resp)
			if got.Page != tc.wantPage {
				t.Errorf("page = %d, want %d", got.Page, tc.wantPage)
			}
			if len(got.Items) != tc.wantLen {
				t.Errorf("items = %d, want %d", len(got.Items), tc.wantLen)
			}
			if got.TotalPages != 2 || got.TotalItems != 12 {
				t.Errorf("meta totalPages=%d totalItems=%d, want 2/12", got.TotalPages, got.TotalItems)
			}
		})
	}
}

// Spec: FR-6 "page 1 when there are no favorites, with totalPages = 0";
// Edge "Favorites view with zero saved articles" (API side of the empty state)
func TestListEndpoint_EmptyReturnsPage1TotalPages0(t *testing.T) {
	app := newApp(newFakeStore())

	for _, q := range []string{"", "?page=1", "?page=5"} {
		resp := do(t, app, http.MethodGet, "/favorites"+q)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%q: status = %d, want 200", q, resp.StatusCode)
		}
		got := decode[pageJSON](t, resp)
		if got.Page != 1 || got.TotalPages != 0 || got.TotalItems != 0 || got.PageSize != 10 {
			t.Errorf("%q: meta = {page:%d totalPages:%d totalItems:%d pageSize:%d}, want {1 0 0 10}",
				q, got.Page, got.TotalPages, got.TotalItems, got.PageSize)
		}
		if got.Items == nil || len(got.Items) != 0 {
			t.Errorf("%q: items = %v, want empty array (not null)", q, got.Items)
		}
	}
}

// ---------------------------------------------------------------------------
// FR-7: GET /favorites/urls
// ---------------------------------------------------------------------------

// Spec: FR-7, AC "returns a url→id map of all favorites"
func TestURLsEndpoint_ReturnsURLToIDMap(t *testing.T) {
	store := newFakeStore()
	app := newApp(store)
	ids := seed(t, app, 15) // more than one page: the map must not be paginated

	resp := do(t, app, http.MethodGet, "/favorites/urls")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := decode[map[string]int64](t, resp)
	if len(got) != 15 {
		t.Fatalf("map has %d entries, want 15", len(got))
	}
	for i, id := range ids {
		url := samplePick(i + 1)["url"]
		if got[url] != id {
			t.Errorf("map[%q] = %d, want %d", url, got[url], id)
		}
	}
}

// Spec: FR-7 (empty list → empty object, so the frontend can index it safely)
func TestURLsEndpoint_EmptyReturnsEmptyObject(t *testing.T) {
	app := newApp(newFakeStore())

	resp := do(t, app, http.MethodGet, "/favorites/urls")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(raw)) != "{}" {
		t.Errorf("body = %q, want {}", raw)
	}
}

// ---------------------------------------------------------------------------
// Inputs & Outputs: store failures → 500 { error }
// ---------------------------------------------------------------------------

// Spec: Inputs & Outputs — every endpoint responds 500 { error } on a store failure
func TestEndpoints_StoreErrorReturns500(t *testing.T) {
	store := newFakeStore()
	store.err = errors.New("db down")
	app := newApp(store)

	cases := []struct {
		name string
		call func() *http.Response
	}{
		{"POST /favorites", func() *http.Response { return postJSON(t, app, "/favorites", samplePick(1)) }},
		{"DELETE /favorites/:id", func() *http.Response { return do(t, app, http.MethodDelete, "/favorites/1") }},
		{"GET /favorites", func() *http.Response { return do(t, app, http.MethodGet, "/favorites?page=1") }},
		{"GET /favorites/urls", func() *http.Response { return do(t, app, http.MethodGet, "/favorites/urls") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := tc.call()
			if resp.StatusCode != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", resp.StatusCode)
			}
			if e := decode[errorJSON](t, resp); e.Error == "" {
				t.Errorf(`body missing non-empty "error" field`)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration tests against real Postgres (skipped unless TEST_DATABASE_URL is set)
// ---------------------------------------------------------------------------

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TABLE IF EXISTS favorites`)
		db.Close()
	})
	_, _ = db.Exec(`DROP TABLE IF EXISTS favorites`)
	return db
}

// Spec: FR-1, AC "favorites table exists with id, source, title, url (unique), reason"
func TestPostgresStore_CreatesTableWithUniqueURL_Integration(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := NewPostgresStore(ctx, db); err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_name = 'favorites'`)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		cols[c] = true
	}
	for _, want := range []string{"id", "source", "title", "url", "reason"} {
		if !cols[want] {
			t.Errorf("favorites table missing column %q (have %v)", want, cols)
		}
	}

	// url must be unique at the DB level (NFR: concurrency).
	_, err = db.ExecContext(ctx, `INSERT INTO favorites (source, title, url, reason) VALUES ('s','t','https://u.test/1','r')`)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO favorites (source, title, url, reason) VALUES ('s','t','https://u.test/1','r')`)
	if err == nil {
		t.Errorf("second insert with same url succeeded; want unique violation")
	}

	// Calling it again on an existing table must be a no-op (CREATE TABLE IF NOT EXISTS).
	if _, err := NewPostgresStore(ctx, db); err != nil {
		t.Errorf("NewPostgresStore on existing table: %v", err)
	}
}

// Spec: NFR concurrency — "uniqueness of url is enforced by the database ... so
// concurrent adds of the same url can't produce duplicates"; FR-2 idempotent Add
func TestPostgresStore_ConcurrentAddSameURLYieldsOneRow_Integration(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	store, err := NewPostgresStore(ctx, db)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	const workers = 20
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	ids := make(chan int64, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, _, err := store.Add(ctx, Favorite{Source: "s", Title: "t", URL: "https://race.test/x", Reason: "r"})
			if err != nil {
				errs <- err
				return
			}
			ids <- f.ID
		}()
	}
	wg.Wait()
	close(errs)
	close(ids)
	for err := range errs {
		t.Errorf("concurrent Add returned error: %v", err)
	}
	seen := map[int64]bool{}
	for id := range ids {
		seen[id] = true
	}
	if len(seen) != 1 {
		t.Errorf("concurrent adds returned %d distinct ids, want 1", len(seen))
	}
	n, err := store.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("rows = %d, want 1", n)
	}
}

// Spec: FR-1/FR-5 through the real store — Add/Delete/List/Count/URLs round trip
func TestPostgresStore_RoundTrip_Integration(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	store, err := NewPostgresStore(ctx, db)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	a, created, err := store.Add(ctx, Favorite{Source: "Medium", Title: "A", URL: "https://rt.test/a", Reason: "ra"})
	if err != nil || !created || a.ID == 0 {
		t.Fatalf("Add a: f=%+v created=%v err=%v", a, created, err)
	}
	b, created, err := store.Add(ctx, Favorite{Source: "Dev.to", Title: "B", URL: "https://rt.test/b", Reason: "rb"})
	if err != nil || !created || b.ID <= a.ID {
		t.Fatalf("Add b: f=%+v created=%v err=%v", b, created, err)
	}
	again, created, err := store.Add(ctx, Favorite{Source: "Dev.to", Title: "B2", URL: "https://rt.test/b", Reason: "rb2"})
	if err != nil || created || again.ID != b.ID || again.Title != "B" {
		t.Errorf("duplicate Add: f=%+v created=%v err=%v; want existing row %d, created=false", again, created, err, b.ID)
	}

	list, err := store.List(ctx, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Errorf("List = %+v, want [b, a] (id desc)", list)
	}
	if n, _ := store.Count(ctx); n != 2 {
		t.Errorf("Count = %d, want 2", n)
	}
	urls, err := store.URLs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if urls["https://rt.test/a"] != a.ID || urls["https://rt.test/b"] != b.ID {
		t.Errorf("URLs = %v", urls)
	}

	if err := store.Delete(ctx, a.ID); err != nil {
		t.Errorf("Delete: %v", err)
	}
	if err := store.Delete(ctx, a.ID); err != nil {
		t.Errorf("repeated Delete must be nil, got %v", err)
	}
	if n, _ := store.Count(ctx); n != 1 {
		t.Errorf("Count after delete = %d, want 1", n)
	}
}

// Spec: FR-8, Edge "Manual Run now overwrites today's digest", AC "POST /digest/run
// leaves the favorites table unchanged". Exercised via the digest store's Save
// (what /digest/run persists) against the same database.
func TestDigestSave_DoesNotTouchFavorites_Integration(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	store, err := NewPostgresStore(ctx, db)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM digests WHERE date = '2099-01-01'`) })

	before, _, err := store.Add(ctx, Favorite{Source: "Hacker News", Title: "Keep me", URL: "https://keep.test/1", Reason: "r"})
	if err != nil {
		t.Fatal(err)
	}

	dstore, err := digest.NewPostgresStore(ctx, db)
	if err != nil {
		t.Fatalf("digest.NewPostgresStore: %v", err)
	}
	day := time.Date(2099, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < 2; i++ { // first run, then an overwriting re-run
		if err := dstore.Save(ctx, digest.Digest{Date: day, Picks: []digest.Pick{
			{Source: "Hacker News", Title: "Keep me", URL: "https://keep.test/1", Reason: "changed"},
		}}); err != nil {
			t.Fatalf("digest Save %d: %v", i, err)
		}
	}

	after, err := store.List(ctx, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0] != before {
		t.Errorf("favorites after digest runs = %+v, want unchanged [%+v]", after, before)
	}
}
