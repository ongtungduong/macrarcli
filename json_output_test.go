package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/nwaples/rardecode/v2"
	"github.com/ongtungduong/macrarcli/internal/rarutil"
)

// TestReportJSON_Shape verifies the JSON summary structure, per-result fields,
// and the exit code returned for a mixed success/failure batch.
func TestReportJSON_Shape(t *testing.T) {
	results := []rarutil.Result{
		{Job: rarutil.Job{Src: "a.rar", Dst: "a.zip"}, Err: nil},
		{Job: rarutil.Job{Src: "b.rar", Dst: "b.zip"}, Err: errors.New("boom")},
	}

	var buf bytes.Buffer
	code := reportJSON(results, &buf)

	if code != 4 {
		t.Errorf("exit code = %d, want 4 (an uncategorized failure is present)", code)
	}

	var got struct {
		Mode      string `json:"mode"`
		Succeeded int    `json:"succeeded"`
		Failed    int    `json:"failed"`
		Results   []struct {
			Src   string `json:"src"`
			Dst   string `json:"dst"`
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}

	if got.Mode != "extract" {
		t.Errorf("mode = %q, want \"extract\"", got.Mode)
	}
	if got.Succeeded != 1 || got.Failed != 1 {
		t.Errorf("summary counts = %d ok / %d failed, want 1/1", got.Succeeded, got.Failed)
	}
	if len(got.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(got.Results))
	}
	if !got.Results[0].OK || got.Results[0].Error != "" {
		t.Errorf("result[0] = %+v, want ok with no error", got.Results[0])
	}
	if got.Results[1].OK || got.Results[1].Error != "boom" {
		t.Errorf("result[1] = %+v, want not-ok with error 'boom'", got.Results[1])
	}
}

// TestReportJSON_AllSuccess returns exit code 0 when nothing failed.
func TestReportJSON_AllSuccess(t *testing.T) {
	results := []rarutil.Result{
		{Job: rarutil.Job{Src: "a.rar", Dst: "a.zip"}, Err: nil},
	}
	var buf bytes.Buffer
	if code := reportJSON(results, &buf); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// TestReportTestJSON_ModeAndNoDst confirms --test --json tags mode "test" and
// omits dst (test writes nothing, so there is no destination to report).
func TestReportTestJSON_ModeAndNoDst(t *testing.T) {
	results := []rarutil.Result{{Job: rarutil.Job{Src: "a.rar"}, Err: nil}}
	var buf bytes.Buffer
	if code := reportTestJSON(results, &buf); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	var got struct {
		Mode    string `json:"mode"`
		Results []struct {
			Dst string `json:"dst"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if got.Mode != "test" {
		t.Errorf("mode = %q, want \"test\"", got.Mode)
	}
	if len(got.Results) != 1 || got.Results[0].Dst != "" {
		t.Errorf("dst = %q, want empty (test writes nothing)", got.Results[0].Dst)
	}
}

// TestReportJSON_PasswordAndChecksumClassification proves the JSON path uses
// the same errors.Is classification as the human path: a rardecode.ErrBadPassword
// aggregates to exit 2, and it takes precedence over an ErrBadFileChecksum
// present in the same batch (the documented 2 > 3 > 4 precedence).
func TestReportJSON_PasswordAndChecksumClassification(t *testing.T) {
	results := []rarutil.Result{
		{Job: rarutil.Job{Src: "a.rar"}, Err: fmt.Errorf("open rar %q: %w", "a.rar", rardecode.ErrBadPassword)},
		{Job: rarutil.Job{Src: "b.rar"}, Err: fmt.Errorf("read rar entry: %w", rardecode.ErrBadFileChecksum)},
	}
	var buf bytes.Buffer
	if code := reportJSON(results, &buf); code != 2 {
		t.Errorf("exit code = %d, want 2 (password error takes precedence over checksum)", code)
	}
}
