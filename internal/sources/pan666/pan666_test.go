package pan666

import (
	"context"
	"net/http"
	"net/http/httptest"
	"pansou/internal/core"
	"testing"
)

func TestSearchConvertsPostsToItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("filter[q]") != "three body" {
			t.Errorf("query = %q", r.URL.Query().Get("filter[q]"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"d1","attributes":{"title":"Three Body","createdAt":"2026-01-02T03:04:05Z"},"relationships":{"mostRelevantPost":{"data":{"id":"p1"}}}}],"included":[{"id":"p1","attributes":{"contentHtml":"<p>https://pan.quark.cn/s/abc</p>"}}]}`))
	}))
	defer server.Close()
	items, err := (Source{Client: server.Client(), Endpoint: server.URL}).Search(context.Background(), core.Query{Text: "three body"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].URL != "https://pan.quark.cn/s/abc" || items[0].Source != "pan666" {
		t.Fatalf("unexpected items: %#v", items)
	}
}
