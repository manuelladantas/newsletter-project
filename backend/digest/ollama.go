package digest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	ollamaModel       = "qwen2.5:7b-instruct-q4_K_M"
	ollamaTemperature = 0.1
	maxPicksPerSource = 3

	// systemPrompt is ported verbatim from the n8n flow.
	systemPrompt = `You are curating a developer's daily digest. You will receive a numbered list of article titles from one site. Pick the 3 most relevant to the user's interests, ranked best first. Respond ONLY with JSON: {"picks": [{"id": <int>, "reason": "<short string>"}]} with exactly 3 entries.`
)

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Format   string          `json:"format"`
	Stream   bool            `json:"stream"`
	Options  ollamaOptions   `json:"options"`
	Messages []ollamaMessage `json:"messages"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
}

type ollamaChatReply struct {
	Message ollamaMessage `json:"message"`
}

func rankArticles(ctx context.Context, client *http.Client, ollamaURL, profile string, articles []Article) ([]Pick, error) {
	if len(articles) == 0 {
		return []Pick{}, nil
	}

	reqBody, err := json.Marshal(ollamaChatRequest{
		Model:   ollamaModel,
		Format:  "json",
		Stream:  false,
		Options: ollamaOptions{Temperature: ollamaTemperature},
		Messages: []ollamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("User interests: %s\n\nArticles:\n%s", profile, FormatNumberedList(articles))},
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(ollamaURL, "/")+"/api/chat", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var chat ollamaChatReply
	if err := json.Unmarshal(body, &chat); err != nil {
		return nil, fmt.Errorf("ollama: decode envelope: %w", err)
	}
	return ParsePicks(chat.Message.Content, articles)
}

func ParsePicks(raw string, articles []Article) ([]Pick, error) {
	var parsed struct {
		Picks []struct {
			ID     int    `json:"id"`
			Reason string `json:"reason"`
		} `json:"picks"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("ollama: invalid picks json: %w", err)
	}
	if parsed.Picks == nil {
		return nil, errors.New("ollama: response has no picks array")
	}

	picks := make([]Pick, 0, maxPicksPerSource)
	for _, p := range parsed.Picks {
		if p.ID < 1 || p.ID > len(articles) {
			continue
		}
		a := articles[p.ID-1]
		picks = append(picks, Pick{Source: a.Source, Title: a.Title, URL: a.URL, Reason: p.Reason})
		if len(picks) == maxPicksPerSource {
			break
		}
	}
	return picks, nil
}
