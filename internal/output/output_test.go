package output

import (
	"bytes"
	"pansou/internal/core"
	"testing"
)

func TestNDJSONWritesOneItemPerLine(t *testing.T) {
	var b bytes.Buffer
	err := Write(&b, "ndjson", core.Result{Items: []core.Item{{Title: "a"}, {Title: "b"}}})
	if err != nil || bytes.Count(b.Bytes(), []byte("\n")) != 2 {
		t.Fatalf("%q %v", b.String(), err)
	}
}
