//go:build linux

package db

import (
	"os"
	"syscall"
)

// statTupleFromSys reads the local POSIX stat tuple exposed by os.FileInfo.Sys(). A FileInfo
// without a *syscall.Stat_t yields an UNKNOWN tuple (fail closed).
func statTupleFromSys(info os.FileInfo) StatTuple {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return StatTuple{}
	}
	return StatTuple{
		Known:   true,
		Size:    info.Size(),
		MtimeNs: st.Mtim.Nano(),
		CtimeNs: st.Ctim.Nano(),
		// Bit-preserving reinterpretation: the inode is only compared for equality, and
		// SQLite INTEGER cannot hold a uint64 with the high bit set.
		Inode: int64(st.Ino), //nolint:gosec // equality-only identifier, see above
	}
}
