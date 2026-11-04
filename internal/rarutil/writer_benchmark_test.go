package rarutil

import (
	"fmt"
	"testing"
)

// Each iteration resolves a whole archive's duplicates, exposing quadratic
// rescanning that a benchmark of one fresh emitter per entry would miss.
func BenchmarkResolveNameDuplicates(b *testing.B) {
	for _, entries := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(entries), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				e := newFsEmitterRoot(nil, limits{}, nil)
				for i := 0; i < entries; i++ {
					if _, err := e.resolveName("f.txt", "f.txt"); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
