package digest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

type SourceKind string

const (
	SourceDevto SourceKind = "devto"
	SourceRSS   SourceKind = "rss"
)

type Source struct {
	Name string
	Kind SourceKind
	URL  string
}

type Article struct {
	Source string `json:"source"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Score  int    `json:"score"`
}

type Pick struct {
	Source string `json:"source"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

type Digest struct {
	Date  time.Time `json:"date"`
	Picks []Pick    `json:"picks"`
}

var ErrNoDigest = errors.New("digest: no digest stored")

type Store interface {
	Save(ctx context.Context, d Digest) error
	Latest(ctx context.Context) (Digest, error)
}

type Config struct {
	OllamaURL       string
	InterestProfile string
}

const interestProfile = "Interested in: backend systems, distributed systems, javascript, golang, go, self-hosting, homelab, Kubernetes internals, database, system design, architecture, frontend posts. Not interested in: generic AI hype posts, career advice."

func ConfigFromEnv() (Config, error) {
	url := os.Getenv("OLLAMA_URL")
	if url == "" {
		return Config{}, errors.New("digest: OLLAMA_URL is not set")
	}
	return Config{OllamaURL: url, InterestProfile: interestProfile}, nil
}

func DefaultSources() []Source {
	return []Source{
		{Name: "Dev.to", Kind: SourceDevto, URL: "https://dev.to/api/articles?top=1&per_page=15"},
		{Name: "Medium", Kind: SourceRSS, URL: "https://medium.com/feed/tag/programming"},
		{Name: "ByteByteGo", Kind: SourceRSS, URL: "https://blog.bytebytego.com/feed"},
		{Name: "Hacker News", Kind: SourceRSS, URL: "https://hnrss.org/frontpage?count=15"},
	}
}

type Logger interface {
	Printf(format string, v ...any)
}

type Pipeline struct {
	Sources         []Source
	OllamaURL       string
	InterestProfile string
	HTTPClient      *http.Client
	Store           Store
	Now             func() time.Time
	Logger          Logger
}

func (p *Pipeline) Run(ctx context.Context) (Digest, error) {
	d := Digest{Date: p.now(), Picks: []Pick{}}

	for _, src := range p.Sources {
		articles, err := fetchArticles(ctx, p.client(), src)
		if err != nil {
			p.logf("digest: skipping source %s: fetch failed: %v", src.Name, err)
			continue
		}
		picks, err := rankArticles(ctx, p.client(), p.OllamaURL, p.InterestProfile, articles)
		if err != nil {
			p.logf("digest: skipping source %s: ranking failed: %v", src.Name, err)
			continue
		}
		d.Picks = append(d.Picks, picks...)
	}

	if err := p.Store.Save(ctx, d); err != nil {
		return Digest{}, fmt.Errorf("digest: save: %w", err)
	}
	return d, nil
}

func (p *Pipeline) client() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return http.DefaultClient
}

func (p *Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Pipeline) logf(format string, v ...any) {
	if p.Logger != nil {
		p.Logger.Printf(format, v...)
	}
}
