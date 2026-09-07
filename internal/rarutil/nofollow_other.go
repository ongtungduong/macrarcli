//go:build !unix

package rarutil

// noFollowFlag is a no-op on non-unix platforms (no portable O_NOFOLLOW).
const noFollowFlag = 0
