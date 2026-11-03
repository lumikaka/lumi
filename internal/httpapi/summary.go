package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"net/http"
	"strconv"
)

func summaryPage[T any](c echo.Context, items []T) error {
	limit := 50
	var err error
	if text := c.QueryParam("limit"); text != "" {
		limit, err = strconv.Atoi(text)
		if err != nil || limit < 1 || limit > 100 {
			return NewError(400, "invalid_pagination", "分页范围无效", "limit 必须为 1–100。", err)
		}
	}
	if c.QueryParam("before") != "" && c.QueryParam("after") != "" {
		return NewError(400, "invalid_pagination", "游标冲突", "before 和 after 不能同时提供。", nil)
	}
	start := 0
	for _, key := range []string{"after", "before"} {
		if text := c.QueryParam(key); text != "" {
			offset, err := strconv.Atoi(text)
			if err != nil || offset < 0 || offset > len(items) {
				return NewError(400, "invalid_pagination", "游标超出范围", "请从首组重新读取。", err)
			}
			start = offset
			if key == "before" {
				start = max(0, offset-limit)
			}
		}
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return err
	}
	snapshot := fmt.Sprintf("%x", sha256.Sum256(encoded))
	end := min(start+limit, len(items))
	return Success(c, http.StatusOK, map[string]any{"items": items[start:end], "snapshot": snapshot, "total": len(items), "cursor_pagination": map[string]any{"per_page": limit, "prev_cursor": strconv.Itoa(start), "next_cursor": strconv.Itoa(end), "has_more": end < len(items)}})
}
