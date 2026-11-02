//go:build unix

package resources

import "syscall"

// diskUsage returns free and total bytes for the filesystem that holds path.
func diskUsage(path string) (free, total uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	blockSize := uint64(stat.Bsize)
	return stat.Bavail * blockSize, stat.Blocks * blockSize, nil
}
