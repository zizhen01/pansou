package core

import (
	"testing"
	"time"
)

func TestDedupeKeepsMoreCompleteItem(t *testing.T) {
	items := Dedupe([]Item{{Title: "old", URL: "https://pan.quark.cn/s/a?utm=x"}, {Title: "better", URL: "https://pan.quark.cn/s/a", Password: "1234"}})
	if len(items) != 1 || items[0].Title != "better" || items[0].Type != "quark" {
		t.Fatalf("unexpected items: %#v", items)
	}
}
func TestFilterAndRank(t *testing.T) {
	now := time.Now()
	items := FilterAndRank([]Item{{Title: "other", URL: "https://pan.baidu.com/s/a"}, {Title: "三体 全集", URL: "https://pan.quark.cn/s/a", PublishedAt: now}}, Query{Text: "三体", Types: []string{"quark"}})
	if len(items) != 1 || items[0].Type != "quark" {
		t.Fatalf("unexpected: %#v", items)
	}
}
