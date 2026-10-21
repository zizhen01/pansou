// Package core contains the transport-independent search domain.
package core

import (
	"net/url"
	"sort"
	"strings"
	"time"
)

type Query struct {
	Text     string
	Sources  []string
	Types    []string
	Limit    int
	Timeout  time.Duration
	NoRank   bool
	NoDedupe bool
}

type Item struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Password    string    `json:"password,omitempty"`
	Source      string    `json:"source"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Type        string    `json:"type"`
}

type Warning struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}
type Result struct {
	Items    []Item        `json:"items"`
	Warnings []Warning     `json:"warnings,omitempty"`
	Elapsed  time.Duration `json:"elapsed"`
}

func Normalize(item Item) Item {
	item.URL = CanonicalURL(item.URL)
	if item.Type == "" {
		item.Type = DetectType(item.URL)
	}
	item.Title = strings.TrimSpace(item.Title)
	return item
}

func CanonicalURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	u.Fragment = ""
	// Keep only query keys that commonly carry a share password. This makes tracking
	// parameters from different sources collapse to the same result.
	q := u.Query()
	kept := url.Values{}
	for _, key := range []string{"pwd", "password", "passcode"} {
		if value := q.Get(key); value != "" {
			kept.Set(key, value)
		}
	}
	u.RawQuery = kept.Encode()
	u.Host = strings.ToLower(u.Host)
	return u.String()
}

func DetectType(raw string) string {
	host := strings.ToLower(raw)
	switch {
	case strings.Contains(host, "pan.quark.cn"):
		return "quark"
	case strings.Contains(host, "alipan.com"), strings.Contains(host, "aliyundrive.com"):
		return "aliyun"
	case strings.Contains(host, "pan.baidu.com"):
		return "baidu"
	case strings.Contains(host, "cloud.189.cn"):
		return "tianyi"
	case strings.Contains(host, "drive.uc.cn"):
		return "uc"
	case strings.Contains(host, "123pan.com"):
		return "123"
	case strings.Contains(host, "115.com"):
		return "115"
	case strings.Contains(host, "pan.xunlei.com"):
		return "xunlei"
	case strings.HasPrefix(host, "magnet:"):
		return "magnet"
	default:
		return "other"
	}
}

func Dedupe(items []Item) []Item {
	best := make(map[string]Item, len(items))
	order := make([]string, 0, len(items))
	for _, item := range items {
		item = Normalize(item)
		key := item.URL
		if key == "" {
			key = item.Source + "\x00" + strings.ToLower(item.Title)
		}
		old, exists := best[key]
		if !exists {
			best[key] = item
			order = append(order, key)
		} else if completeness(item) > completeness(old) {
			best[key] = item
		}
	}
	out := make([]Item, 0, len(order))
	for _, key := range order {
		out = append(out, best[key])
	}
	return out
}

func FilterAndRank(items []Item, q Query) []Item {
	types := make(map[string]bool, len(q.Types))
	for _, t := range q.Types {
		if t = strings.TrimSpace(strings.ToLower(t)); t != "" {
			types[t] = true
		}
	}
	out := make([]Item, 0, len(items))
	for _, item := range items {
		item = Normalize(item)
		if len(types) == 0 || types[item.Type] {
			out = append(out, item)
		}
	}
	if !q.NoRank {
		sort.SliceStable(out, func(i, j int) bool { return score(out[i], q.Text) > score(out[j], q.Text) })
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out
}

func completeness(item Item) int {
	score := len(item.Title)
	if item.URL != "" {
		score += 1000
	}
	if item.Password != "" {
		score += 20
	}
	if !item.PublishedAt.IsZero() {
		score += 10
	}
	return score
}
func score(item Item, query string) int {
	result := completeness(item)
	if !item.PublishedAt.IsZero() {
		days := int(time.Since(item.PublishedAt).Hours() / 24)
		if days < 0 {
			days = 0
		}
		result += max(0, 365-days)
	}
	if strings.Contains(strings.ToLower(item.Title), strings.ToLower(strings.TrimSpace(query))) {
		result += 500
	}
	return result
}
