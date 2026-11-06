//go:build windows

package flasher

import (
	"strings"

	"golang.org/x/sys/windows"
)

func freeBytes(dir string) (int64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return int64(free), nil
}

func volumeRoot(dir string) string {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return ""
	}
	buf := make([]uint16, 1024)
	if err := windows.GetVolumePathName(p, &buf[0], uint32(len(buf))); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// fsIsFAT: the folder is on a FAT/FAT32 drive (no file can reach 4 GB).
func fsIsFAT(dir string) bool {
	root := volumeRoot(dir)
	if root == "" {
		return false
	}
	r, _ := windows.UTF16PtrFromString(root)
	fs := make([]uint16, 64)
	if err := windows.GetVolumeInformation(r, nil, 0, nil, nil, nil, &fs[0], uint32(len(fs))); err != nil {
		return false
	}
	n := strings.ToUpper(windows.UTF16ToString(fs))
	return n == "FAT" || n == "FAT32"
}

// diskID names the drive that holds dir ("" if unknown).
func diskID(dir string) string {
	return strings.ToLower(volumeRoot(dir))
}
