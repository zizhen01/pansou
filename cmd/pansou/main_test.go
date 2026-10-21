package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"pansou/internal/core"
)

func TestQueryCanPrecedeFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"search", "three body", "--source", "missing"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "unknown source") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestSourcesPrintsDefaultChannels(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"sources"}, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Aliyun_4K_Movies") || !strings.Contains(stdout.String(), "Quark_Movies") {
		t.Fatalf("unexpected sources output: %q", stdout.String())
	}
}

func TestRecentRequiresOneChannel(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"recent"}, &stdout, &stderr); code != 1 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestWarningsAreSummarizedUnlessVerbose(t *testing.T) {
	warnings := []core.Warning{{Source: "a", Message: "remote failed"}, {Source: "b", Message: context.DeadlineExceeded.Error()}}
	var summary bytes.Buffer
	printWarnings(&summary, warnings, false)
	if !strings.Contains(summary.String(), "1 failed, 1 timed out") || strings.Contains(summary.String(), "remote failed") {
		t.Fatalf("unexpected summary: %q", summary.String())
	}
	var details bytes.Buffer
	printWarnings(&details, warnings, true)
	if !strings.Contains(details.String(), "remote failed") {
		t.Fatalf("missing details: %q", details.String())
	}
}
