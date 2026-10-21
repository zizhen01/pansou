package output

import (
	"encoding/json"
	"fmt"
	"io"
	"pansou/internal/core"
	"strings"
)

func Write(w io.Writer, format string, result core.Result) error {
	switch strings.ToLower(format) {
	case "json":
		return json.NewEncoder(w).Encode(result)
	case "ndjson":
		enc := json.NewEncoder(w)
		for _, item := range result.Items {
			if err := enc.Encode(item); err != nil {
				return err
			}
		}
		return nil
	case "text", "":
		for i, item := range result.Items {
			date := ""
			if !item.PublishedAt.IsZero() {
				date = item.PublishedAt.Format("2006-01-02")
			}
			if _, err := fmt.Fprintf(w, "%d\t[%s]\t%s\t%s\tsource=%s %s\n", i+1, item.Type, item.Title, item.URL, item.Source, date); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q (use text, json, or ndjson)", format)
	}
}
