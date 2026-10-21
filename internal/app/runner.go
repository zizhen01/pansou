package app

import (
	"context"
	"fmt"
	"pansou/internal/core"
	"sync"
	"time"
)

type Source interface {
	Name() string
	Search(context.Context, core.Query) ([]core.Item, error)
}
type Runner struct {
	Sources  []Source
	Parallel int
}

func (r Runner) Run(ctx context.Context, q core.Query) core.Result {
	started := time.Now()
	selected, warnings := r.selectSources(q.Sources)
	if len(selected) == 0 {
		return core.Result{Warnings: append(warnings, core.Warning{Message: "no sources selected"}), Elapsed: time.Since(started)}
	}
	parallel := r.Parallel
	if parallel <= 0 {
		parallel = 4
	}
	if parallel > len(selected) {
		parallel = len(selected)
	}
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var all []core.Item
	for _, source := range selected {
		source := source
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				warnings = append(warnings, core.Warning{Source: source.Name(), Message: ctx.Err().Error()})
				mu.Unlock()
				return
			}
			defer func() { <-sem }()
			items, err := source.Search(ctx, q)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				warnings = append(warnings, core.Warning{Source: source.Name(), Message: err.Error()})
				return
			}
			all = append(all, items...)
		}()
	}
	wg.Wait()
	if !q.NoDedupe {
		all = core.Dedupe(all)
	}
	return core.Result{Items: core.FilterAndRank(all, q), Warnings: warnings, Elapsed: time.Since(started)}
}
func (r Runner) selectSources(names []string) ([]Source, []core.Warning) {
	wanted := map[string]bool{}
	for _, n := range names {
		wanted[n] = true
	}
	selectAll := len(wanted) == 0
	var selected []Source
	for _, s := range r.Sources {
		if selectAll || wanted[s.Name()] {
			selected = append(selected, s)
			delete(wanted, s.Name())
		}
	}
	var warnings []core.Warning
	for name := range wanted {
		warnings = append(warnings, core.Warning{Source: name, Message: fmt.Sprintf("unknown source %q", name)})
	}
	return selected, warnings
}
