//go:build !windows

package flasher

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func freeBytes(dir string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

// fsIsFAT: the folder is on a FAT (FAT32/vfat) filesystem, which cannot hold
// a file of 4 GB or more.
func fsIsFAT(dir string) bool {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return false
	}
	return st.Type == 0x4d44 // MSDOS_SUPER_MAGIC
}

// diskID names the filesystem that holds dir ("" if unknown).
func diskID(dir string) string {
	var st unix.Stat_t
	if err := unix.Stat(dir, &st); err != nil {
		return ""
	}
	return fmt.Sprintf("dev:%d", st.Dev)
}
