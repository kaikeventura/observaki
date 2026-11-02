//go:build !unix

package resources

func diskUsage(path string) (free, total uint64, err error) {
	return 0, 0, errUnsupported
}
