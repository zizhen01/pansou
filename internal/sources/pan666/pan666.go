// Package pan666 is a small, stateless adapter for Pan666's public discussion API.
package pan666

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io"
	"net/http"
	"net/url"
	"pansou/internal/core"
	"regexp"
	"strings"
	"time"
)

const endpoint = "https://pan666.net/api/discussions"

var linkRE = regexp.MustCompile(`https?://[^\s<>"']+`)

type Source struct {
	Client   *http.Client
	Endpoint string // Optional override for tests or a self-hosted compatible endpoint.
}

func (s Source) Name() string { return "pan666" }
func (s Source) Search(ctx context.Context, q core.Query) ([]core.Item, error) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	baseURL := s.Endpoint
	if baseURL == "" {
		baseURL = endpoint
	}
	u := baseURL + "?filter[q]=" + url.QueryEscape(q.Text) + "&include=mostRelevantPost&page[limit]=50"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "pansou-cli/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pan666 returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var payload response
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode pan666 response: %w", err)
	}
	posts := map[string]string{}
	for _, p := range payload.Included {
		posts[p.ID] = p.Attributes.ContentHTML
	}
	var out []core.Item
	for _, d := range payload.Data {
		html := posts[d.Relationships.MostRelevantPost.Data.ID]
		text := htmlToText(html)
		when, _ := time.Parse(time.RFC3339, d.Attributes.CreatedAt)
		for _, raw := range linkRE.FindAllString(text, -1) {
			raw = strings.TrimRight(raw, ".,;:)")
			out = append(out, core.Item{Title: strings.TrimSpace(d.Attributes.Title), URL: raw, Source: "pan666", PublishedAt: when})
		}
	}
	return out, nil
}
func htmlToText(raw string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		return raw
	}
	return doc.Text()
}

type response struct {
	Data     []discussion `json:"data"`
	Included []post       `json:"included"`
}
type discussion struct {
	ID         string `json:"id"`
	Attributes struct {
		Title     string `json:"title"`
		CreatedAt string `json:"createdAt"`
	} `json:"attributes"`
	Relationships struct {
		MostRelevantPost struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"mostRelevantPost"`
	} `json:"relationships"`
}
type post struct {
	ID         string `json:"id"`
	Attributes struct {
		ContentHTML string `json:"contentHtml"`
	} `json:"attributes"`
}
