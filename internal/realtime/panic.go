package realtime

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

func runSafely(component string, function func()) {
	defer recoverPanic(component, nil)
	function()
}

func recoverPanic(component string, cleanup func()) {
	recovered := recover()
	if recovered == nil {
		return
	}
	slog.Error("realtime background panic recovered",
		"component", component,
		"panic", fmt.Sprint(recovered),
		"stack", string(debug.Stack()),
	)
	if cleanup != nil {
		func() {
			defer func() {
				if secondary := recover(); secondary != nil {
					slog.Error("secondary realtime cleanup panic recovered",
						"component", component,
						"panic", fmt.Sprint(secondary),
						"stack", string(debug.Stack()),
					)
				}
			}()
			cleanup()
		}()
	}
}
