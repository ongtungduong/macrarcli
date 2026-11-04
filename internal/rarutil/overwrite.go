package rarutil

import (
	"errors"
	"fmt"
	"io/fs"
)

// OverwritePolicy controls existing destination files. The default aborts.
type OverwritePolicy int

const (
	OverwriteFail OverwritePolicy = iota
	OverwriteOverwrite
	OverwriteSkip
	OverwriteRename
)

// A resolver caches the next suffix per original name for this transaction.
// Probing errors are propagated, rather than mistaken for endless collisions.
type overwriteResolver struct {
	lstat   func(string) (fs.FileInfo, error)
	claimed map[string]bool
	next    map[string]int
}

func newOverwriteResolver(lstat func(string) (fs.FileInfo, error)) *overwriteResolver {
	return &overwriteResolver{lstat: lstat, claimed: map[string]bool{}, next: map[string]int{}}
}
func (r *overwriteResolver) occupied(name string) (bool, error) {
	if r.claimed[name] {
		return true, nil
	}
	_, err := r.lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
func (r *overwriteResolver) resolve(name string, policy OverwritePolicy) (string, bool, error) {
	occupied, err := r.occupied(name)
	if err != nil {
		return "", false, fmt.Errorf("stat %q: %w", name, err)
	}
	if !occupied {
		r.claimed[name] = true
		return name, false, nil
	}
	switch policy {
	case OverwriteOverwrite:
		r.claimed[name] = true
		return name, false, nil
	case OverwriteSkip:
		return "", true, nil
	case OverwriteRename:
		k := r.next[name]
		if k < 1 {
			k = 1
		}
		for ; ; k++ {
			candidate := dedupVariant(name, k)
			occupied, err := r.occupied(candidate)
			if err != nil {
				return "", false, fmt.Errorf("stat %q: %w", candidate, err)
			}
			if !occupied {
				r.next[name] = k + 1
				r.claimed[candidate] = true
				return candidate, false, nil
			}
		}
	default:
		return "", false, fmt.Errorf("output exists (use --overwrite to replace, --skip to skip, or --rename to keep both): %s", name)
	}
}
