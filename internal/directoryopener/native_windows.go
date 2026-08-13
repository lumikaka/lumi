//go:build windows

package directoryopener

import "context"

func openNativeDirectory(ctx context.Context, rootPath string) error {
	return runNativeCommand(ctx, "explorer.exe", rootPath)
}
