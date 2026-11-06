//go:build !darwin && !linux && !windows

package vacsimport

import "fmt"

func publishDirectory(source, target string) error {
	return fmt.Errorf("exclusive project publication is supported on macOS, Linux and Windows")
}
