// Package legacy adapts the project's existing plugin registry to CLI sources.
package legacy

import (
	"context"
	"io"
	"log"
	"sort"
	"sync"

	"pansou/config"
	"pansou/internal/core"
	_ "pansou/internal/legacyplugins"
	"pansou/model"
	"pansou/plugin"
	"pansou/util"
)

var initOnce sync.Once

func Initialize() {
	initOnce.Do(func() {
		// Legacy plugins use the process-wide standard logger for request-level
		// debugging. CLI diagnostics are emitted centrally after aggregation.
		log.SetOutput(io.Discard)
		config.Init()
		util.InitHTTPClient()
		plugin.InitAsyncPluginSystem()
	})
}

func Sources() []Source {
	Initialize()
	plugins := plugin.GetRegisteredPlugins()
	sources := make([]Source, 0, len(plugins))
	for _, registered := range plugins {
		sources = append(sources, Source{Plugin: registered})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Name() < sources[j].Name() })
	return sources
}

type Source struct{ Plugin plugin.AsyncSearchPlugin }

func (s Source) Name() string { return s.Plugin.Name() }
func (s Source) Search(ctx context.Context, q core.Query) ([]core.Item, error) {
	results := make(chan []model.SearchResult, 1)
	errs := make(chan error, 1)
	go func() {
		items, err := s.Plugin.Search(q.Text, nil)
		if err != nil {
			errs <- err
			return
		}
		results <- items
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errs:
		return nil, err
	case result := <-results:
		return convert(result, s.Name()), nil
	}
}

func convert(results []model.SearchResult, source string) []core.Item {
	out := make([]core.Item, 0)
	for _, result := range results {
		for _, link := range result.Links {
			out = append(out, core.Item{Title: result.Title, URL: link.URL, Password: link.Password, Source: source, PublishedAt: result.Datetime, Tags: result.Tags, Type: link.Type})
		}
	}
	return out
}
