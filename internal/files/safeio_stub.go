//go:build !linux

package files

// safeio_stub provides no-op stubs for non-Linux platforms (macOS development).
// The safeio functions are Linux-only (openat2 syscall); on macOS the panel
// is not intended to run, only to compile for development iteration.

import (
	"errors"
	"io"
	"os"
)

var errSafeIOLinuxOnly = errors.New("safeio: openat2 is Linux-only, this platform is not supported")

func openAt2Beneath(home, rel string, flags int, mode uint32) (*os.File, error) {
	return nil, errSafeIOLinuxOnly
}

func chmodBeneath(home, rel string, mode uint32) error {
	return errSafeIOLinuxOnly
}

func writeBeneath(home, rel string, data []byte, createMode uint32, sk string) error {
	return errSafeIOLinuxOnly
}

func createExclBeneath(home, rel, sk string) error {
	return errSafeIOLinuxOnly
}

func copyStreamBeneath(home, rel string, src io.Reader, sk string) (int64, error) {
	return 0, errSafeIOLinuxOnly
}

func mkdirAllBeneath(home, rel, sk string) error {
	return errSafeIOLinuxOnly
}

func renameBeneath(home, oldRel, newRel, sk string) error {
	return errSafeIOLinuxOnly
}

func removeAllBeneath(home, rel string) error {
	return errSafeIOLinuxOnly
}

func copyTreeBeneath(home, srcRel, dstRel, sk string) error {
	return errSafeIOLinuxOnly
}


func isDirBeneath(home, rel string) (bool, error) {
	return false, errSafeIOLinuxOnly
}
