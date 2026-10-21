// Package tg adapts Telegram's public channel search pages to core.Source.
package tg

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"pansou/internal/core"
	"pansou/model"
	"pansou/util"
	"strings"
	"time"
)

type Source struct {
	Channels []string
	Client   *http.Client
}

type RecentMessage struct {
	Title       string      `json:"title"`
	Content     string      `json:"content"`
	PublishedAt time.Time   `json:"published_at"`
	Channel     string      `json:"channel"`
	Links       []core.Item `json:"links,omitempty"`
}

func (s Source) Name() string { return "tg" }
func (s Source) Search(ctx context.Context, q core.Query) ([]core.Item, error) {
	if len(s.Channels) == 0 {
		return nil, fmt.Errorf("tg source has no channels configured")
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	var out []core.Item
	var firstErr error
	for _, channel := range s.Channels {
		page := util.BuildSearchURL(channel, q.Text, "")
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, page, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "pansou-cli/0.1")
		resp, err := client.Do(req)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if firstErr == nil {
				firstErr = readErr
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			if firstErr == nil {
				firstErr = fmt.Errorf("telegram returned HTTP %d", resp.StatusCode)
			}
			continue
		}
		results, _, err := util.ParseSearchResults(string(body), channel)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, convert(results, "tg:"+channel, q.Text)...)
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// Recent returns messages shown on a channel's public page, newest first.
// Telegram only exposes public channels through this endpoint.
func (s Source) Recent(ctx context.Context, channel string, limit int) ([]RecentMessage, error) {
	results, err := s.fetch(ctx, channel, "")
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	messages := make([]RecentMessage, 0, len(results))
	for _, result := range results {
		messages = append(messages, RecentMessage{
			Title:       result.Title,
			Content:     strings.TrimSpace(result.Content),
			PublishedAt: result.Datetime,
			Channel:     channel,
			Links:       convert([]model.SearchResult{result}, "tg:"+channel, ""),
		})
	}
	return messages, nil
}

func (s Source) fetch(ctx context.Context, channel, query string) ([]model.SearchResult, error) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	page := util.BuildSearchURL(channel, query, "")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, page, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "pansou-cli/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram returned HTTP %d", resp.StatusCode)
	}
	results, _, err := util.ParseSearchResults(string(body), channel)
	return results, err
}
func convert(results []model.SearchResult, source, query string) []core.Item {
	out := make([]core.Item, 0)
	for _, result := range results {
		if !matches(result, query) {
			continue
		}
		for _, link := range result.Links {
			raw := link.URL
			if decoded, err := url.QueryUnescape(raw); err == nil {
				raw = decoded
			}
			out = append(out, core.Item{Title: result.Title, URL: raw, Password: link.Password, Source: source, PublishedAt: result.Datetime, Type: link.Type, Tags: result.Tags})
		}
	}
	return out
}

// Telegram may return a channel's recent posts when its server-side q filter
// is ignored. Keep only entries that actually match every query token.
func matches(result model.SearchResult, query string) bool {
	haystack := strings.ToLower(result.Title + "\n" + result.Content)
	for _, token := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}
