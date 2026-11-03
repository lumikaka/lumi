package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSummaryCursorRangesAndSnapshot(t *testing.T) {
	items := make([]map[string]any, 205)
	for i := range items {
		items[i] = map[string]any{"uuid": fmt.Sprintf("01989abc-def0-7000-8000-%012d", i+1), "title": "Card"}
	}
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.GET("/summaries", func(c echo.Context) error { return summaryPage(c, items) })
	read := func(query string) (int, map[string]any) {
		t.Helper()
		r := httptest.NewRecorder()
		e.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/summaries"+query, nil))
		var v map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return r.Code, v
	}
	_, first := read("?limit=100")
	d := first["data"].(map[string]any)
	snapshot := d["snapshot"]
	if len(d["items"].([]any)) != 100 || d["total"] != float64(205) || d["cursor_pagination"].(map[string]any)["has_more"] != true {
		t.Fatal(d)
	}
	_, second := read("?limit=100&after=100")
	s := second["data"].(map[string]any)
	if s["snapshot"] != snapshot || s["items"].([]any)[0].(map[string]any)["uuid"] == d["items"].([]any)[0].(map[string]any)["uuid"] {
		t.Fatal("cursor repeated rows")
	}
	_, last := read("?limit=100&after=200")
	l := last["data"].(map[string]any)
	if len(l["items"].([]any)) != 5 || l["cursor_pagination"].(map[string]any)["has_more"] != false {
		t.Fatal(l)
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?after=-1", "?after=206", "?after=1&before=2", "?after=bad"} {
		status, v := read(query)
		if status != 400 || v["success"] != false || v["data"] != nil {
			t.Fatal(query, status, v)
		}
	}
	items[0]["title"] = "Changed"
	_, changed := read("?limit=100")
	if changed["data"].(map[string]any)["snapshot"] == snapshot {
		t.Fatal("changed facts did not change snapshot")
	}
}
