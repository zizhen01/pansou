package app

import (
	"context"
	"errors"
	"pansou/internal/core"
	"testing"
)

type fake struct {
	name  string
	items []core.Item
	err   error
}

func (f fake) Name() string                                            { return f.name }
func (f fake) Search(context.Context, core.Query) ([]core.Item, error) { return f.items, f.err }
func TestRunnerReturnsPartialResults(t *testing.T) {
	r := Runner{Sources: []Source{fake{"ok", []core.Item{{Title: "x", URL: "https://pan.quark.cn/s/a"}}, nil}, fake{"bad", nil, errors.New("down")}}}
	got := r.Run(context.Background(), core.Query{})
	if len(got.Items) != 1 || len(got.Warnings) != 1 {
		t.Fatalf("%#v", got)
	}
}

func TestRunnerOnlyRunsRequestedSource(t *testing.T) {
	r := Runner{Sources: []Source{
		fake{"requested", []core.Item{{Title: "yes", URL: "https://pan.quark.cn/s/a"}}, nil},
		fake{"unrequested", nil, errors.New("must not run")},
	}}
	got := r.Run(context.Background(), core.Query{Sources: []string{"requested"}})
	if len(got.Items) != 1 || len(got.Warnings) != 0 {
		t.Fatalf("unexpected result: %#v", got)
	}
}
