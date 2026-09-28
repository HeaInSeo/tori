//go:build !linux

package db

import "os"

// statTupleFromSys: only the local POSIX (Linux) profile is supported (I5F0). Every other
// platform observes an UNKNOWN tuple, which fails closed.
func statTupleFromSys(os.FileInfo) StatTuple {
	return StatTuple{}
}
