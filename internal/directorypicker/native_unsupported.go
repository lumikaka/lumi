//go:build !darwin && !windows && !linux

package directorypicker

import "context"

func pickNativeDirectory(_ context.Context, _ string) (string, error) {
	return "", ErrUnavailable
}
