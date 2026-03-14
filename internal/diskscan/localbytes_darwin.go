//go:build darwin

package diskscan

import (
	"os"
	"syscall"
)

const darwinFlagDataless = 0x40000000

func localSize(info os.FileInfo) uint64 {
	if isDataless(info) {
		return 0
	}
	return statBlocksSize(info)
}

func isDataless(info os.FileInfo) bool {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Flags&darwinFlagDataless != 0
	}
	return false
}
