package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

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

	if code != 1 {
		t.Errorf("exit code = %d, want 1 (a failure is present)", code)
	}

	var got struct {
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
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
