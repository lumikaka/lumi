//go:build linux

package directoryopener

import "context"

func openNativeDirectory(ctx context.Context, rootPath string) error {
	return runNativeCommand(ctx, "xdg-open", rootPath)
}
