//go:build windows

package filesystem

import "golang.org/x/sys/windows"

// availableFreeBytes returns free space at path using GetDiskFreeSpaceEx.
func availableFreeBytes(path string) int64 {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}
	var freeBytesAvailable uint64
	if err := windows.GetDiskFreeSpaceEx(pathPtr, &freeBytesAvailable, nil, nil); err != nil {
		return 0
	}
	return int64(freeBytesAvailable)
}