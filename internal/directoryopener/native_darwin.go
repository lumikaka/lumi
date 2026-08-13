//go:build darwin

package directoryopener

import "context"

func openNativeDirectory(ctx context.Context, rootPath string) error {
	return runNativeCommand(ctx, "open", rootPath)
}
