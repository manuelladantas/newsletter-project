package digest

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

const maxArticlesPerSource = 15

func fetchArticles(ctx context.Context, client *http.Client, src Source) ([]Article, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	switch src.Kind {
	case SourceDevto:
		return NormalizeDevto(src.Name, body)
	case SourceRSS:
		return NormalizeRSS(src.Name, body)
	default:
		return nil, fmt.Errorf("unknown source kind %q", src.Kind)
	}
}

func NormalizeDevto(source string, body []byte) ([]Article, error) {
	var raw []struct {
		Title                  string `json:"title"`
		URL                    string `json:"url"`
		PositiveReactionsCount int    `json:"positive_reactions_count"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("dev.to: %w", err)
	}
	articles := make([]Article, 0, len(raw))
	for _, a := range raw {
		articles = append(articles, Article{Source: source, Title: a.Title, URL: a.URL, Score: a.PositiveReactionsCount})
	}
	sort.SliceStable(articles, func(i, j int) bool { return articles[i].Score > articles[j].Score })
	return capArticles(articles), nil
}

func NormalizeRSS(source string, body []byte) ([]Article, error) {
	var feed struct {
		Channel struct {
			Items []struct {
				Title string `xml:"title"`
				Link  string `xml:"link"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("rss: %w", err)
	}
	articles := make([]Article, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		articles = append(articles, Article{Source: source, Title: it.Title, URL: it.Link})
	}
	return capArticles(articles), nil
}

func capArticles(articles []Article) []Article {
	if len(articles) > maxArticlesPerSource {
		return articles[:maxArticlesPerSource]
	}
	return articles
}

func FormatNumberedList(articles []Article) string {
	lines := make([]string, 0, len(articles))
	for i, a := range articles {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, a.Title))
	}
	return strings.Join(lines, "\n")
}
