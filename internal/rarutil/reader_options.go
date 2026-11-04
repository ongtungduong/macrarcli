package rarutil

import (
	"fmt"

	"github.com/nwaples/rardecode/v2"
)

// DefaultMaxDictionaryBytes bounds the decoder dictionary for every backend
// operation, including callers that use zero-value Options. It is a per-reader
// dictionary limit, not a cap on total process memory or decompressed output.
const DefaultMaxDictionaryBytes int64 = 256 << 20

// readerOptions keeps password and dictionary policy identical across opens.
func (o Options) readerOptions() ([]rardecode.Option, error) {
	if o.MaxDictionaryBytes < 0 {
		return nil, fmt.Errorf("MaxDictionaryBytes must not be negative")
	}
	maxDictionary := o.MaxDictionaryBytes
	if maxDictionary == 0 {
		maxDictionary = DefaultMaxDictionaryBytes
	}
	opts := []rardecode.Option{rardecode.MaxDictionarySize(maxDictionary)}
	if o.Password != "" {
		opts = append(opts, rardecode.Password(o.Password))
	}
	return opts, nil
}
