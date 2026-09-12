package digest

// Tests for the Daily Article Digest feature.
// Every test traces back to specs/daily-article-digest/spec.md (FR-n = Functional
// Requirement n, "Edge:" = Edge Cases table row, "AC:" = Acceptance Criteria item,
// "NFR:" = Non-Functional Requirement).
//
// These tests are written before the implementation exists (red step of TDD) and
// are expected to fail to compile until the digest package is implemented.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ---------------------------------------------------------------------------
// Test helpers (test-only; no production logic lives here)
// ---------------------------------------------------------------------------

// fakeStore is an in-memory Store keeping one Digest per calendar date.
type fakeStore struct {
	mu      sync.Mutex
	byDate  map[string]Digest
	saves   int
	lastKey string
}

func newFakeStore() *fakeStore {
	return &fakeStore{byDate: map[string]Digest{}}
}

func dateKey(t time.Time) string { return t.Format("2006-01-02") }

func (s *fakeStore) Save(_ context.Context, d Digest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	s.lastKey = dateKey(d.Date)
	s.byDate[s.lastKey] = d
	return nil
}

func (s *fakeStore) Latest(_ context.Context) (Digest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest Digest
	found := false
	for _, d := range s.byDate {
		if !found || d.Date.After(latest.Date) {
			latest = d
			found = true
		}
	}
	if !found {
		return Digest{}, ErrNoDigest
	}
	return latest, nil
}

// recordingTransport records every outbound request host and delegates to the
// default transport (httptest servers are reachable through it).
type recordingTransport struct {
	mu    sync.Mutex
	hosts []string
	next  http.RoundTripper
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.hosts = append(r.hosts, req.URL.Host)
	r.mu.Unlock()
	return r.next.RoundTrip(req)
}

// devtoJSON builds a Dev.to API-style response with n articles.
func devtoJSON(prefix string, n int) []byte {
	items := make([]map[string]any, 0, n)
	for i := 1; i <= n; i++ {
		items = append(items, map[string]any{
			"title":                    fmt.Sprintf("%s article %d", prefix, i),
			"url":                      fmt.Sprintf("https://dev.to/%s/%d", prefix, i),
			"positive_reactions_count": i,
		})
	}
	b, _ := json.Marshal(items)
	return b
}

// rssXML builds a minimal RSS 2.0 feed with n items.
func rssXML(prefix string, n int) []byte {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel><title>feed</title>`)
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, `<item><title>%s article %d</title><link>https://example.com/%s/%d</link></item>`, prefix, i, prefix, i)
	}
	sb.WriteString(`</channel></rss>`)
	return []byte(sb.String())
}

// ollamaChatResponse wraps a picks payload in the /api/chat envelope
// ({"message":{"content":"<json string>"}}).
func ollamaChatResponse(content string) []byte {
	b, _ := json.Marshal(map[string]any{
		"message": map[string]any{"role": "assistant", "content": content},
		"done":    true,
	})
	return b
}

// okPicks is a valid 3-pick Ollama content for ids 1,2,3.
const okPicks = `{"picks":[{"id":1,"reason":"r1"},{"id":2,"reason":"r2"},{"id":3,"reason":"r3"}]}`

// ollamaRequest is the shape of a captured Ollama /api/chat request body.
type ollamaRequest struct {
	Model   string `json:"model"`
	Format  string `json:"format"`
	Stream  bool   `json:"stream"`
	Options struct {
		Temperature float64 `json:"temperature"`
	} `json:"options"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// fakeOllama returns an httptest server that answers /api/chat with the given
// content and captures every request body.
func fakeOllama(t *testing.T, content string) (*httptest.Server, func() []ollamaRequest) {
	t.Helper()
	var mu sync.Mutex
	var reqs []ollamaRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req ollamaRequest
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		reqs = append(reqs, req)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(ollamaChatResponse(content))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []ollamaRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]ollamaRequest(nil), reqs...)
	}
}

// staticServer serves a fixed body with the given status and counts hits.
func staticServer(t *testing.T, status int, body []byte) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return hits }
}

// fourHealthySources spins up one Dev.to-style and three RSS-style servers,
// each with 15 articles, and returns Sources pointing at them.
func fourHealthySources(t *testing.T) []Source {
	t.Helper()
	devto, _ := staticServer(t, 200, devtoJSON("devto", 15))
	medium, _ := staticServer(t, 200, rssXML("medium", 15))
	bbg, _ := staticServer(t, 200, rssXML("bytebytego", 15))
	hn, _ := staticServer(t, 200, rssXML("hn", 15))
	return []Source{
		{Name: "Dev.to", Kind: SourceDevto, URL: devto.URL},
		{Name: "Medium", Kind: SourceRSS, URL: medium.URL},
		{Name: "ByteByteGo", Kind: SourceRSS, URL: bbg.URL},
		{Name: "Hacker News", Kind: SourceRSS, URL: hn.URL},
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 9, 12, 9, 0, 0, 0, time.Local)
}

func newPipeline(t *testing.T, sources []Source, ollamaURL string, store Store) (*Pipeline, *bytes.Buffer) {
	t.Helper()
	logBuf := &bytes.Buffer{}
	return &Pipeline{
		Sources:         sources,
		OllamaURL:       ollamaURL,
		InterestProfile: "Go, distributed systems, databases",
		HTTPClient:      &http.Client{Timeout: 5 * time.Second},
		Store:           store,
		Now:             fixedNow,
		Logger:          log.New(logBuf, "", 0),
	}, logBuf
}

func countBySource(picks []Pick) map[string]int {
	m := map[string]int{}
	for _, p := range picks {
		m[p.Source]++
	}
	return m
}

// ---------------------------------------------------------------------------
// FR-1 / AC: cron fires at 9am server-local time
// ---------------------------------------------------------------------------

func TestNextRunAt_Before9amIsToday9am(t *testing.T) {
	now := time.Date(2026, 9, 12, 7, 30, 0, 0, time.Local)
	want := time.Date(2026, 9, 12, 9, 0, 0, 0, time.Local)
	if got := NextRunAt(now); !got.Equal(want) {
		t.Fatalf("NextRunAt(%v) = %v, want %v", now, got, want)
	}
}

func TestNextRunAt_After9amIsTomorrow9am(t *testing.T) {
	now := time.Date(2026, 9, 12, 9, 0, 1, 0, time.Local)
	want := time.Date(2026, 9, 13, 9, 0, 0, 0, time.Local)
	if got := NextRunAt(now); !got.Equal(want) {
		t.Fatalf("NextRunAt(%v) = %v, want %v", now, got, want)
	}
}

func TestNextRunAt_UsesLocalTimezone(t *testing.T) {
	now := time.Date(2026, 9, 12, 7, 0, 0, 0, time.Local)
	got := NextRunAt(now)
	if got.Location().String() != time.Local.String() {
		t.Fatalf("NextRunAt location = %s, want local (%s)", got.Location(), time.Local)
	}
	if got.Hour() != 9 || got.Minute() != 0 {
		t.Fatalf("NextRunAt = %v, want 09:00 local", got)
	}
}

// ---------------------------------------------------------------------------
// FR-3 / AC: the four sources with the same parameters as n8nflow.json
// ---------------------------------------------------------------------------

func TestDefaultSources_MatchesN8nFlow(t *testing.T) {
	want := []Source{
		{Name: "Dev.to", Kind: SourceDevto, URL: "https://dev.to/api/articles?top=1&per_page=15"},
		{Name: "Medium", Kind: SourceRSS, URL: "https://medium.com/feed/tag/programming"},
		{Name: "ByteByteGo", Kind: SourceRSS, URL: "https://blog.bytebytego.com/feed"},
		{Name: "Hacker News", Kind: SourceRSS, URL: "https://hnrss.org/frontpage?count=15"},
	}
	got := DefaultSources()
	if len(got) != len(want) {
		t.Fatalf("DefaultSources() has %d sources, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("DefaultSources()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// FR-4: normalize into {source,title,url,score}, cap at 15, numbered list
// ---------------------------------------------------------------------------

func TestNormalizeDevto(t *testing.T) {
	body := []byte(`[
		{"title":"Low","url":"https://dev.to/a/low","positive_reactions_count":2},
		{"title":"High","url":"https://dev.to/a/high","positive_reactions_count":50},
		{"title":"NoScore","url":"https://dev.to/a/noscore"}
	]`)
	got, err := NormalizeDevto("Dev.to", body)
	if err != nil {
		t.Fatalf("NormalizeDevto error: %v", err)
	}
	// n8n flow sorts Dev.to by reactions desc and defaults missing score to 0
	want := []Article{
		{Source: "Dev.to", Title: "High", URL: "https://dev.to/a/high", Score: 50},
		{Source: "Dev.to", Title: "Low", URL: "https://dev.to/a/low", Score: 2},
		{Source: "Dev.to", Title: "NoScore", URL: "https://dev.to/a/noscore", Score: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d articles, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("article[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestNormalizeRSS(t *testing.T) {
	got, err := NormalizeRSS("Medium", rssXML("m", 2))
	if err != nil {
		t.Fatalf("NormalizeRSS error: %v", err)
	}
	want := []Article{
		{Source: "Medium", Title: "m article 1", URL: "https://example.com/m/1", Score: 0},
		{Source: "Medium", Title: "m article 2", URL: "https://example.com/m/2", Score: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d articles, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("article[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestNormalize_CapsAt15(t *testing.T) {
	t.Run("devto", func(t *testing.T) {
		got, err := NormalizeDevto("Dev.to", devtoJSON("d", 20))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 15 {
			t.Fatalf("got %d articles, want 15", len(got))
		}
	})
	t.Run("rss", func(t *testing.T) {
		got, err := NormalizeRSS("Medium", rssXML("m", 20))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 15 {
			t.Fatalf("got %d articles, want 15", len(got))
		}
	})
}

func TestFormatNumberedList(t *testing.T) {
	articles := []Article{
		{Title: "First"},
		{Title: "Second"},
		{Title: "Third"},
	}
	want := "1. First\n2. Second\n3. Third"
	if got := FormatNumberedList(articles); got != want {
		t.Fatalf("FormatNumberedList = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FR-5: Ollama request shape (JSON mode, model, temperature, profile + list)
// ---------------------------------------------------------------------------

func TestRun_SendsExpectedOllamaRequest(t *testing.T) {
	sources := fourHealthySources(t)
	ollama, requests := fakeOllama(t, okPicks)
	p, _ := newPipeline(t, sources, ollama.URL, newFakeStore())

	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	reqs := requests()
	if len(reqs) != 4 {
		t.Fatalf("Ollama called %d times, want once per source (4)", len(reqs))
	}
	for i, r := range reqs {
		if r.Model != "qwen2.5:7b-instruct-q4_K_M" {
			t.Errorf("req[%d].model = %q, want qwen2.5:7b-instruct-q4_K_M", i, r.Model)
		}
		if r.Format != "json" {
			t.Errorf("req[%d].format = %q, want json", i, r.Format)
		}
		if r.Stream {
			t.Errorf("req[%d].stream = true, want false", i)
		}
		if r.Options.Temperature != 0.1 {
			t.Errorf("req[%d].options.temperature = %v, want 0.1", i, r.Options.Temperature)
		}
		if len(r.Messages) != 2 || r.Messages[0].Role != "system" || r.Messages[1].Role != "user" {
			t.Fatalf("req[%d].messages = %+v, want [system, user]", i, r.Messages)
		}
		system := r.Messages[0].Content
		if !strings.Contains(system, `"picks"`) || !strings.Contains(system, "3") {
			t.Errorf("req[%d] system prompt should request 3 picks in a {\"picks\":[...]} shape, got %q", i, system)
		}
		user := r.Messages[1].Content
		if !strings.Contains(user, p.InterestProfile) {
			t.Errorf("req[%d] user message missing interest profile; got %q", i, user)
		}
		if !strings.Contains(user, "1. ") || !strings.Contains(user, "15. ") {
			t.Errorf("req[%d] user message should contain numbered list 1..15; got %q", i, user)
		}
	}
}

// ---------------------------------------------------------------------------
// FR-6 / Edge: parsing picks and mapping back to articles
// ---------------------------------------------------------------------------

func threeArticles(source string) []Article {
	return []Article{
		{Source: source, Title: "A", URL: "https://x/a", Score: 1},
		{Source: source, Title: "B", URL: "https://x/b", Score: 2},
		{Source: source, Title: "C", URL: "https://x/c", Score: 3},
	}
}

func TestParsePicks_MapsIdsToArticles(t *testing.T) {
	raw := `{"picks":[{"id":2,"reason":"best"},{"id":3,"reason":"good"},{"id":1,"reason":"ok"}]}`
	got, err := ParsePicks(raw, threeArticles("Src"))
	if err != nil {
		t.Fatalf("ParsePicks error: %v", err)
	}
	want := []Pick{
		{Source: "Src", Title: "B", URL: "https://x/b", Reason: "best"},
		{Source: "Src", Title: "C", URL: "https://x/c", Reason: "good"},
		{Source: "Src", Title: "A", URL: "https://x/a", Reason: "ok"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d picks, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pick[%d] = %+v, want %+v (order must be preserved: ranked best-first)", i, got[i], want[i])
		}
	}
}

// Edge: Ollama returns malformed/invalid JSON -> that source's picks are skipped.
func TestParsePicks_MalformedJSONReturnsError(t *testing.T) {
	cases := []string{
		`not json at all`,
		`{"picks": "nope"}`,
		`{"picks": [{"id": "one", "reason": "x"}]}`,
		``,
	}
	for _, raw := range cases {
		if _, err := ParsePicks(raw, threeArticles("Src")); err == nil {
			t.Errorf("ParsePicks(%q) expected error, got nil", raw)
		}
	}
}

// Edge: invalid/out-of-range ids are ignored; valid picks remain.
func TestParsePicks_DropsOutOfRangeIds(t *testing.T) {
	raw := `{"picks":[{"id":0,"reason":"zero"},{"id":2,"reason":"ok"},{"id":99,"reason":"too big"},{"id":-1,"reason":"neg"}]}`
	got, err := ParsePicks(raw, threeArticles("Src"))
	if err != nil {
		t.Fatalf("ParsePicks error: %v", err)
	}
	if len(got) != 1 || got[0].Title != "B" || got[0].Reason != "ok" {
		t.Fatalf("got %+v, want only the valid pick for article B", got)
	}
}

// Edge: wrong pick count (more than 3) is clamped to 3.
func TestParsePicks_ClampsToThree(t *testing.T) {
	articles := []Article{
		{Source: "S", Title: "1"}, {Source: "S", Title: "2"}, {Source: "S", Title: "3"},
		{Source: "S", Title: "4"}, {Source: "S", Title: "5"},
	}
	raw := `{"picks":[{"id":1,"reason":"a"},{"id":2,"reason":"b"},{"id":3,"reason":"c"},{"id":4,"reason":"d"},{"id":5,"reason":"e"}]}`
	got, err := ParsePicks(raw, articles)
	if err != nil {
		t.Fatalf("ParsePicks error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d picks, want 3 (clamped)", len(got))
	}
	for i, title := range []string{"1", "2", "3"} {
		if got[i].Title != title {
			t.Errorf("pick[%d].Title = %q, want %q (first three, best-first)", i, got[i].Title, title)
		}
	}
}

// Edge: fewer than 3 picks are kept as-is rather than rejected.
func TestParsePicks_FewerThanThreeKept(t *testing.T) {
	raw := `{"picks":[{"id":3,"reason":"only one"}]}`
	got, err := ParsePicks(raw, threeArticles("Src"))
	if err != nil {
		t.Fatalf("ParsePicks error: %v", err)
	}
	if len(got) != 1 || got[0].Title != "C" {
		t.Fatalf("got %+v, want single pick for C", got)
	}
}

// ---------------------------------------------------------------------------
// FR-7 / AC: merge picks from all sources, up to 3 per source with reasons
// ---------------------------------------------------------------------------

func TestRun_MergesPicksFromAllSources(t *testing.T) {
	sources := fourHealthySources(t)
	ollama, _ := fakeOllama(t, okPicks)
	p, _ := newPipeline(t, sources, ollama.URL, newFakeStore())

	d, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(d.Picks) != 12 {
		t.Fatalf("got %d picks, want 12 (3 per source x 4 sources): %+v", len(d.Picks), d.Picks)
	}
	counts := countBySource(d.Picks)
	for _, s := range sources {
		if counts[s.Name] != 3 {
			t.Errorf("source %q has %d picks, want 3", s.Name, counts[s.Name])
		}
	}
	for i, pk := range d.Picks {
		if pk.Title == "" || pk.URL == "" || pk.Reason == "" || pk.Source == "" {
			t.Errorf("pick[%d] has empty field(s): %+v", i, pk)
		}
	}
}

// ---------------------------------------------------------------------------
// FR-8 / AC: persisted to the store keyed by the run's date
// ---------------------------------------------------------------------------

func TestRun_SavesDigestKeyedByRunDate(t *testing.T) {
	sources := fourHealthySources(t)
	ollama, _ := fakeOllama(t, okPicks)
	store := newFakeStore()
	p, _ := newPipeline(t, sources, ollama.URL, store)

	d, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if store.saves != 1 {
		t.Fatalf("store.Save called %d times, want 1", store.saves)
	}
	if store.lastKey != dateKey(fixedNow()) {
		t.Fatalf("saved under date %q, want %q", store.lastKey, dateKey(fixedNow()))
	}
	saved := store.byDate[store.lastKey]
	if len(saved.Picks) != len(d.Picks) {
		t.Fatalf("saved %d picks, returned %d; they should match", len(saved.Picks), len(d.Picks))
	}
	if dateKey(d.Date) != dateKey(fixedNow()) {
		t.Fatalf("returned digest date %v, want run date %v", d.Date, fixedNow())
	}
}

// ---------------------------------------------------------------------------
// FR-9 / Edge / AC: re-run on the same day overwrites stored results
// ---------------------------------------------------------------------------

func TestRun_SecondRunSameDayOverwrites(t *testing.T) {
	sources := fourHealthySources(t)
	store := newFakeStore()

	first, _ := fakeOllama(t, okPicks)
	p, _ := newPipeline(t, sources, first.URL, store)
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("first Run error: %v", err)
	}

	// Second run (same Now/date) with different picks must replace the first.
	second, _ := fakeOllama(t, `{"picks":[{"id":7,"reason":"second run"}]}`)
	p.OllamaURL = second.URL
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("second Run error: %v", err)
	}

	if len(store.byDate) != 1 {
		t.Fatalf("store holds %d dates, want 1 (same-day rerun must overwrite)", len(store.byDate))
	}
	latest, err := store.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(latest.Picks) != 4 {
		t.Fatalf("after rerun got %d picks, want 4 (1 per source from second run)", len(latest.Picks))
	}
	for _, pk := range latest.Picks {
		if pk.Reason != "second run" {
			t.Errorf("pick %+v is from the first run; should have been overwritten", pk)
		}
	}
}

// ---------------------------------------------------------------------------
// FR-11 / Edge / AC: a source fetch failure skips only that source, no retry
// ---------------------------------------------------------------------------

func TestRun_SourceFetchFailureSkipsOnlyThatSource(t *testing.T) {
	t.Run("http 500", func(t *testing.T) {
		sources := fourHealthySources(t)
		failing, hits := staticServer(t, 500, []byte("boom"))
		sources[1] = Source{Name: "Medium", Kind: SourceRSS, URL: failing.URL}
		ollama, _ := fakeOllama(t, okPicks)
		p, _ := newPipeline(t, sources, ollama.URL, newFakeStore())

		d, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("Run should not fail on a single source error, got: %v", err)
		}
		counts := countBySource(d.Picks)
		if counts["Medium"] != 0 {
			t.Errorf("failed source Medium produced %d picks, want 0", counts["Medium"])
		}
		for _, name := range []string{"Dev.to", "ByteByteGo", "Hacker News"} {
			if counts[name] != 3 {
				t.Errorf("source %q has %d picks, want 3 (partial results)", name, counts[name])
			}
		}
		if hits() != 1 {
			t.Errorf("failed source fetched %d times, want exactly 1 (no retry)", hits())
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		sources := fourHealthySources(t)
		dead := httptest.NewServer(http.NotFoundHandler())
		deadURL := dead.URL
		dead.Close()
		sources[0] = Source{Name: "Dev.to", Kind: SourceDevto, URL: deadURL}
		ollama, _ := fakeOllama(t, okPicks)
		p, _ := newPipeline(t, sources, ollama.URL, newFakeStore())

		d, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("Run should not fail on a single source error, got: %v", err)
		}
		if len(d.Picks) != 9 {
			t.Fatalf("got %d picks, want 9 (3 remaining sources x 3)", len(d.Picks))
		}
		if countBySource(d.Picks)["Dev.to"] != 0 {
			t.Errorf("unreachable source still produced picks: %+v", d.Picks)
		}
	})
}

// ---------------------------------------------------------------------------
// FR-12 / Edge / AC: Ollama failure or invalid response skips that source's picks
// ---------------------------------------------------------------------------

func TestRun_OllamaUnreachableSkipsSourcePicks(t *testing.T) {
	sources := fourHealthySources(t)
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	store := newFakeStore()
	p, _ := newPipeline(t, sources, deadURL, store)

	d, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run should complete even if Ollama is unreachable, got: %v", err)
	}
	if len(d.Picks) != 0 {
		t.Fatalf("got %d picks with Ollama down, want 0", len(d.Picks))
	}
	if store.saves != 1 {
		t.Fatalf("run should still persist its (empty) result; Save called %d times", store.saves)
	}
}

func TestRun_OllamaMalformedResponseSkipsSource(t *testing.T) {
	sources := fourHealthySources(t)
	// Respond with garbage for the first source only, valid picks for the rest.
	var mu sync.Mutex
	calls := 0
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			_, _ = w.Write(ollamaChatResponse(`this is not json`))
			return
		}
		_, _ = w.Write(ollamaChatResponse(okPicks))
	}))
	t.Cleanup(ollama.Close)
	p, _ := newPipeline(t, sources, ollama.URL, newFakeStore())

	d, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run should not fail on one malformed Ollama response, got: %v", err)
	}
	if len(d.Picks) != 9 {
		t.Fatalf("got %d picks, want 9 (one source skipped, three x 3 remain)", len(d.Picks))
	}
}

// Edge: wrong pick count / out-of-range ids from Ollama end-to-end through Run.
func TestRun_OllamaInvalidIdsAreDroppedNotFatal(t *testing.T) {
	sources := fourHealthySources(t)
	ollama, _ := fakeOllama(t, `{"picks":[{"id":1,"reason":"ok"},{"id":42,"reason":"bad"},{"id":0,"reason":"bad"}]}`)
	p, _ := newPipeline(t, sources, ollama.URL, newFakeStore())

	d, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(d.Picks) != 4 {
		t.Fatalf("got %d picks, want 4 (one valid pick per source)", len(d.Picks))
	}
	for _, pk := range d.Picks {
		if pk.Reason != "ok" {
			t.Errorf("invalid pick leaked through: %+v", pk)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge / AC: a source with fewer than 3 articles still produces ranked picks
// ---------------------------------------------------------------------------

func TestRun_SourceWithTwoArticlesStillRanked(t *testing.T) {
	sources := fourHealthySources(t)
	small, _ := staticServer(t, 200, rssXML("small", 2))
	sources[2] = Source{Name: "ByteByteGo", Kind: SourceRSS, URL: small.URL}
	// Ollama obeys the "3 picks" prompt literally; id 3 doesn't exist for the small source.
	ollama, requests := fakeOllama(t, okPicks)
	p, _ := newPipeline(t, sources, ollama.URL, newFakeStore())

	d, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	counts := countBySource(d.Picks)
	if counts["ByteByteGo"] != 2 {
		t.Errorf("small source produced %d picks, want 2 (whatever is available)", counts["ByteByteGo"])
	}
	if len(d.Picks) != 11 {
		t.Errorf("got %d picks total, want 11", len(d.Picks))
	}
	// The small source must still have been sent to Ollama for ranking.
	found := false
	for _, r := range requests() {
		if len(r.Messages) == 2 && strings.Contains(r.Messages[1].Content, "small article 1") {
			found = true
		}
	}
	if !found {
		t.Error("source with 2 articles was not sent to Ollama for ranking")
	}
}

// ---------------------------------------------------------------------------
// NFR observability: one log line per skipped source
// ---------------------------------------------------------------------------

func TestRun_LogsEachSkippedSource(t *testing.T) {
	sources := fourHealthySources(t)
	failing, _ := staticServer(t, 503, nil)
	sources[1] = Source{Name: "Medium", Kind: SourceRSS, URL: failing.URL}
	sources[3] = Source{Name: "Hacker News", Kind: SourceRSS, URL: failing.URL}
	ollama, _ := fakeOllama(t, okPicks)
	p, logBuf := newPipeline(t, sources, ollama.URL, newFakeStore())

	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	logs := logBuf.String()
	for _, name := range []string{"Medium", "Hacker News"} {
		if !strings.Contains(logs, name) {
			t.Errorf("expected a log line mentioning skipped source %q; logs:\n%s", name, logs)
		}
	}
	for _, name := range []string{"Dev.to", "ByteByteGo"} {
		if strings.Contains(logs, name) {
			t.Errorf("healthy source %q should not be logged as skipped; logs:\n%s", name, logs)
		}
	}
}

// ---------------------------------------------------------------------------
// NFR configurability / AC: OLLAMA_URL from env, not hardcoded
// ---------------------------------------------------------------------------

func TestConfigFromEnv_ReadsOllamaURL(t *testing.T) {
	t.Setenv("OLLAMA_URL", "http://ollama.internal:11434")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv error: %v", err)
	}
	if cfg.OllamaURL != "http://ollama.internal:11434" {
		t.Fatalf("cfg.OllamaURL = %q, want value from OLLAMA_URL env", cfg.OllamaURL)
	}
	if cfg.InterestProfile == "" {
		t.Fatal("cfg.InterestProfile is empty; spec requires a fixed backend-config profile")
	}
}

func TestConfigFromEnv_MissingOllamaURLIsError(t *testing.T) {
	t.Setenv("OLLAMA_URL", "")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("ConfigFromEnv with empty OLLAMA_URL should error rather than fall back to a hardcoded host")
	}
}

// ---------------------------------------------------------------------------
// AC: no ntfy call remains in the ported pipeline
// ---------------------------------------------------------------------------

func TestRun_MakesNoRequestToNtfy(t *testing.T) {
	sources := fourHealthySources(t)
	ollama, _ := fakeOllama(t, okPicks)
	rt := &recordingTransport{next: http.DefaultTransport}
	p, _ := newPipeline(t, sources, ollama.URL, newFakeStore())
	p.HTTPClient = &http.Client{Transport: rt, Timeout: 5 * time.Second}

	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	for _, h := range rt.hosts {
		if strings.Contains(strings.ToLower(h), "ntfy") {
			t.Fatalf("pipeline made a request to %q; ntfy must be removed entirely", h)
		}
	}
	// 4 source fetches + 4 Ollama calls, nothing else.
	if len(rt.hosts) != 8 {
		t.Fatalf("pipeline made %d outbound requests, want 8 (4 fetch + 4 rank); hosts: %v", len(rt.hosts), rt.hosts)
	}
}

// ---------------------------------------------------------------------------
// FR-2 / AC: manual-trigger endpoint; FR-10 backend half: latest digest endpoint
// (route paths follow the spec's Technical Design examples: POST /digest/run,
// GET /digest/today)
// ---------------------------------------------------------------------------

func newTestApp(t *testing.T, p *Pipeline, store Store) *fiber.App {
	t.Helper()
	app := fiber.New()
	RegisterRoutes(app, p, store)
	return app
}

func decodeDigest(t *testing.T, body io.Reader) Digest {
	t.Helper()
	var d Digest
	if err := json.NewDecoder(body).Decode(&d); err != nil {
		t.Fatalf("decode digest response: %v", err)
	}
	return d
}

func TestRunEndpoint_TriggersPipelineAndReturnsDigest(t *testing.T) {
	sources := fourHealthySources(t)
	ollama, requests := fakeOllama(t, okPicks)
	store := newFakeStore()
	p, _ := newPipeline(t, sources, ollama.URL, store)
	app := newTestApp(t, p, store)

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/digest/run", nil), 10000)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /digest/run status = %d, want 200", resp.StatusCode)
	}
	d := decodeDigest(t, resp.Body)
	if len(d.Picks) != 12 {
		t.Fatalf("response has %d picks, want 12", len(d.Picks))
	}
	if len(requests()) != 4 {
		t.Fatalf("manual trigger ran Ollama %d times, want 4 (same pipeline as cron)", len(requests()))
	}
	if store.saves != 1 {
		t.Fatalf("manual trigger should persist the result; Save called %d times", store.saves)
	}
}

func TestTodayEndpoint_ReturnsLatestStoredDigest(t *testing.T) {
	store := newFakeStore()
	older := Digest{
		Date:  time.Date(2026, 9, 11, 9, 0, 0, 0, time.Local),
		Picks: []Pick{{Source: "Dev.to", Title: "old", URL: "https://x/old", Reason: "old"}},
	}
	newer := Digest{
		Date: time.Date(2026, 9, 12, 9, 0, 0, 0, time.Local),
		Picks: []Pick{
			{Source: "Dev.to", Title: "new", URL: "https://x/new", Reason: "new"},
			{Source: "Medium", Title: "new2", URL: "https://x/new2", Reason: "new2"},
		},
	}
	_ = store.Save(context.Background(), older)
	_ = store.Save(context.Background(), newer)
	p, _ := newPipeline(t, nil, "", store)
	app := newTestApp(t, p, store)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/digest/today", nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /digest/today status = %d, want 200", resp.StatusCode)
	}
	d := decodeDigest(t, resp.Body)
	if dateKey(d.Date) != dateKey(newer.Date) {
		t.Fatalf("returned digest date %v, want latest %v", d.Date, newer.Date)
	}
	if len(d.Picks) != 2 {
		t.Fatalf("returned %d picks, want 2 from the latest digest", len(d.Picks))
	}
	for i, want := range newer.Picks {
		if d.Picks[i] != want {
			t.Errorf("pick[%d] = %+v, want %+v (source, title, url, reason must round-trip)", i, d.Picks[i], want)
		}
	}
}

func TestTodayEndpoint_NoDigestYetReturns404(t *testing.T) {
	store := newFakeStore()
	p, _ := newPipeline(t, nil, "", store)
	app := newTestApp(t, p, store)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/digest/today", nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /digest/today with empty store status = %d, want 404", resp.StatusCode)
	}
}
