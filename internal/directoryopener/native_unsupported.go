//go:build !darwin && !windows && !linux

package directoryopener

import "context"

func openNativeDirectory(_ context.Context, _ string) error {
	return ErrUnavailable
}
