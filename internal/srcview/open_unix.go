//go:build !windows

package srcview

import "golang.org/x/sys/unix"

const nonblockingOpenFlag = unix.O_NONBLOCK
