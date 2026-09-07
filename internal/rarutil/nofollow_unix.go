//go:build unix

package rarutil

import "syscall"

// noFollowFlag makes renameOrCopy's fallback open refuse to follow a symlink
// planted at dst between the caller's existence check and this open — the
// kernel fails the open with ELOOP instead of writing through the link.
const noFollowFlag = syscall.O_NOFOLLOW
