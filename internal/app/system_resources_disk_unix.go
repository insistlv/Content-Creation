//go:build !windows

package app

import "syscall"

func readDiskStats(path string) (adminDiskStats, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return adminDiskStats{}, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	if free > total {
		free = total
	}
	used := total - free
	return adminDiskStats{
		Path:         path,
		TotalBytes:   total,
		UsedBytes:    used,
		FreeBytes:    free,
		UsagePercent: roundedPercent(float64(used), float64(total)),
	}, nil
}
