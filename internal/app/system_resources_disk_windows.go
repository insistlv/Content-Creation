package app

import (
	"syscall"
	"unsafe"
)

var kernel32GetDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func readDiskStats(path string) (adminDiskStats, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return adminDiskStats{}, err
	}
	var freeAvailable, total, free uint64
	result, _, callErr := kernel32GetDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeAvailable)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if result == 0 {
		return adminDiskStats{}, callErr
	}
	if freeAvailable > total {
		freeAvailable = total
	}
	used := total - freeAvailable
	return adminDiskStats{
		Path:         path,
		TotalBytes:   total,
		UsedBytes:    used,
		FreeBytes:    freeAvailable,
		UsagePercent: roundedPercent(float64(used), float64(total)),
	}, nil
}
