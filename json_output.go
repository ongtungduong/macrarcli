package main

import (
	"encoding/json"
	"io"

	"github.com/ongtungduong/macrarcli/internal/rarutil"
)

// jsonResult is the machine-readable form of one archive's outcome, shared by
// extract/flat and test modes (list mode has its own richer per-entry shape
// in list_output.go). Dst is empty and omitted for test mode, which writes
// nothing.
type jsonResult struct {
	Src            string   `json:"src"`
	Dst            string   `json:"dst,omitempty"`
	OK             bool     `json:"ok"`
	SkippedEntries []string `json:"skippedEntries,omitempty"`
	Error          string   `json:"error,omitempty"`
}

// jsonSummary is the top-level --json document for extract/flat/test modes.
type jsonSummary struct {
	Mode      string       `json:"mode"`
	Succeeded int          `json:"succeeded"`
	Skipped   int          `json:"skipped"`
	Failed    int          `json:"failed"`
	Results   []jsonResult `json:"results"`
}

// reportJSON writes a JSON summary of results to w, tagged with mode
// ("extract" or "test"), and returns the aggregate exit code per the 5-code
// scheme (classifyErr/aggregateExit — see main.go).
func reportJSON(results []rarutil.Result, w io.Writer) int {
	return writeJSONSummary(results, "extract", w)
}

// reportTestJSON is reportJSON for --test: same shape, mode "test", Dst
// naturally empty (test writes nothing) and omitted from the output.
func reportTestJSON(results []rarutil.Result, w io.Writer) int {
	return writeJSONSummary(results, "test", w)
}

func writeJSONSummary(results []rarutil.Result, mode string, w io.Writer) int {
	summary := jsonSummary{Mode: mode, Results: make([]jsonResult, 0, len(results))}
	codes := make([]int, len(results))
	for i, r := range results {
		codes[i] = classifyErr(r.Err)
		jr := jsonResult{Src: r.Src, Dst: r.Dst, OK: r.Err == nil, SkippedEntries: r.SkippedEntries}
		switch {
		case r.Err != nil:
			jr.Error = r.Err.Error()
			summary.Failed++
		case len(r.SkippedEntries) > 0:
			summary.Skipped++
		default:
			summary.Succeeded++
		}
		summary.Results = append(summary.Results, jr)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	// Encode failure is effectively impossible for this struct + an os.Stdout
	// writer, but surface it as a runtime error rather than reporting success.
	if err := enc.Encode(summary); err != nil {
		return 4
	}
	return aggregateExit(codes)
}
