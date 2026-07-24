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

func openAt2Beneath(_, _ string, _ int, _ uint32) (*os.File, error) {
	return nil, errSafeIOLinuxOnly
}

func chmodBeneath(_, _ string, _ uint32) error {
	return errSafeIOLinuxOnly
}

func writeBeneath(_, _ string, _ []byte, _ uint32, _ string) error {
	return errSafeIOLinuxOnly
}

func createExclBeneath(_, _, _ string) error {
	return errSafeIOLinuxOnly
}

func copyStreamBeneath(_, _ string, _ io.Reader, _ string) (int64, error) {
	return 0, errSafeIOLinuxOnly
}

func mkdirAllBeneath(_, _, _ string) error {
	return errSafeIOLinuxOnly
}

func renameBeneath(_, _, _, _ string) error {
	return errSafeIOLinuxOnly
}

func removeAllBeneath(_, _ string) error {
	return errSafeIOLinuxOnly
}

func copyTreeBeneath(_, _, _, _ string) error {
	return errSafeIOLinuxOnly
}

func isDirBeneath(_, _ string) (bool, error) {
	return false, errSafeIOLinuxOnly
}

func statBeneath(_, _ string) (os.FileInfo, error) {
	return nil, errSafeIOLinuxOnly
}

func readDirBeneath(_, _ string) ([]os.DirEntry, error) {
	return nil, errSafeIOLinuxOnly
}

func readFileBeneath(_, _ string, _ int64) ([]byte, os.FileInfo, error) {
	return nil, nil, errSafeIOLinuxOnly
}

func openReadBeneath(_, _ string) (*os.File, error) {
	return nil, errSafeIOLinuxOnly
}

func fchownRestoreFd(_ string, _ *os.File, _ string) {}
