//go:build !darwin

package diskscan

import "os"

func localSize(info os.FileInfo) uint64 {
	return statBlocksSize(info)
}

func isDataless(os.FileInfo) bool {
	return false
}
