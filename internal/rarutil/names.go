package rarutil

import (
	"fmt"
	"path"
	"strings"
)

// resolveName applies the duplicate-name guard to a fully-formed staging name
// (files have no trailing slash, directories do). A repeat is suspicious only
// when it arrives from a DIFFERENT raw archive name — i.e. a path that used
// traversal/separators to collide after sanitizing. That is a data-loss/
// overwrite attempt and is rejected. A repeat of the SAME raw name, which
// legitimate multi-volume/recovery archives may re-stream, is preserved by
// renaming to a free " (k)" variant rather than dropped.
func (e *fsEmitter) resolveName(raw, name string) (string, error) {
	if e.flat {
		// Flattening deliberately merges different source directories. It
		// must not conceal a collision already present before flattening.
		source, err := sanitize(raw)
		if err != nil {
			return "", err
		}
		if first, seen := e.sources[source]; seen && first != raw {
			return "", fmt.Errorf("post-sanitize name collision: %q maps to %q", raw, source)
		}
		e.sources[source] = raw
	}
	first, seen := e.origin[name]
	if !seen {
		e.origin[name] = raw
		return name, nil
	}
	if first != raw && !e.flat {
		return "", fmt.Errorf("post-sanitize name collision: %q and a prior entry both map to %q", raw, name)
	}
	k := e.next[name]
	if k == 0 {
		k = 1
	}
	for ; ; k++ {
		cand := dedupVariant(name, k)
		if _, taken := e.origin[cand]; !taken {
			e.origin[cand] = raw
			e.next[name] = k + 1
			return cand, nil
		}
	}
}

// dedupVariant inserts " (k)" before the extension (or before the trailing
// slash for directories): "f.txt" -> "f (1).txt", "d/" -> "d (1)/".
func dedupVariant(name string, k int) string {
	isDir := strings.HasSuffix(name, "/")
	trimmed := strings.TrimSuffix(name, "/")
	ext := path.Ext(trimmed)
	v := fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(trimmed, ext), k, ext)
	if isDir {
		v += "/"
	}
	return v
}
