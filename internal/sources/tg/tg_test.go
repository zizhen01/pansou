package tg

import (
	"testing"

	"pansou/model"
)

func TestMatchesUsesTitleAndContent(t *testing.T) {
	result := model.SearchResult{Title: "unrelated", Content: "The Three Body Problem"}
	if !matches(result, "three body") {
		t.Fatal("expected content match")
	}
	if matches(result, "three dune") {
		t.Fatal("expected all query tokens to be required")
	}
}
