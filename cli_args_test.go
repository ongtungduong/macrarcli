package main

import (
	"testing"

	"github.com/ongtungduong/macrarcli/internal/rarutil"
)

func TestParseSize(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"0", 0, false},
		{"", 0, false},
		{"500", 500, false},
		{"10K", 10 << 10, false},
		{"2m", 2 << 20, false},
		{"1G", 1 << 30, false},
		{"-5", 0, true},
		{"abc", 0, true},
		{"10X", 0, true},
	}
	for _, c := range cases {
		got, err := parseSize(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseSize(%q) = %d, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSize(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestResolveExplicitPassword(t *testing.T) {
	tests := []struct {
		name    string
		flagVal string
		env     string
		want    string
	}{
		{"flag wins over env", "flagpw", "envpw", "flagpw"},
		{"env used when flag empty", "", "envpw", "envpw"},
		{"both empty", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveExplicitPassword(tc.flagVal, func(string) string { return tc.env })
			if got != tc.want {
				t.Errorf("resolveExplicitPassword(%q, env=%q) = %q, want %q", tc.flagVal, tc.env, got, tc.want)
			}
		})
	}
}

func TestResolveMode(t *testing.T) {
	tests := []struct {
		name             string
		flat, list, test bool
		want             cliMode
		wantErr          bool
	}{
		{"default extract", false, false, false, modeExtract, false},
		{"flat", true, false, false, modeFlat, false},
		{"list", false, true, false, modeList, false},
		{"test", false, false, true, modeTest, false},
		{"flat+list conflict", true, true, false, 0, true},
		{"list+test conflict", false, true, true, 0, true},
		{"all three conflict", true, true, true, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveMode(tc.flat, tc.list, tc.test)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveMode(%v,%v,%v) = %v, want error", tc.flat, tc.list, tc.test, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveMode(%v,%v,%v) unexpected error: %v", tc.flat, tc.list, tc.test, err)
			}
			if got != tc.want {
				t.Errorf("resolveMode(%v,%v,%v) = %v, want %v", tc.flat, tc.list, tc.test, got, tc.want)
			}
		})
	}
}

func TestResolveOverwritePolicy(t *testing.T) {
	tests := []struct {
		name                    string
		overwrite, skip, rename bool
		wantPolicy              rarutil.OverwritePolicy
		wantSet                 bool
		wantErr                 bool
	}{
		{"none set", false, false, false, rarutil.OverwriteFail, false, false},
		{"overwrite", true, false, false, rarutil.OverwriteOverwrite, true, false},
		{"skip", false, true, false, rarutil.OverwriteSkip, true, false},
		{"rename", false, false, true, rarutil.OverwriteRename, true, false},
		{"overwrite+skip conflict", true, true, false, 0, false, true},
		{"all three conflict", true, true, true, 0, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			policy, set, err := resolveOverwritePolicy(tc.overwrite, tc.skip, tc.rename)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveOverwritePolicy(%v,%v,%v) = %v/%v, want error", tc.overwrite, tc.skip, tc.rename, policy, set)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveOverwritePolicy(%v,%v,%v) unexpected error: %v", tc.overwrite, tc.skip, tc.rename, err)
			}
			if policy != tc.wantPolicy || set != tc.wantSet {
				t.Errorf("resolveOverwritePolicy(%v,%v,%v) = %v/%v, want %v/%v", tc.overwrite, tc.skip, tc.rename, policy, set, tc.wantPolicy, tc.wantSet)
			}
		})
	}
}

func TestValidateArgs(t *testing.T) {
	tests := []struct {
		name         string
		inputs       []string
		mode         cliMode
		dest         string
		overwriteSet bool
		jobs         int
		wantCode     int
	}{
		{"no inputs", nil, modeExtract, "", false, 1, 1},
		{"ok extract", []string{"a.rar"}, modeExtract, "", false, 1, 0},
		{"dest with list mode rejected", []string{"a.rar"}, modeList, "out", false, 1, 1},
		{"overwrite with test mode rejected", []string{"a.rar"}, modeTest, "", true, 1, 1},
		{"jobs below one", []string{"a.rar"}, modeExtract, "", false, 0, 1},
		{"wrong extension", []string{"a.txt"}, modeExtract, "", false, 1, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateArgs(tc.inputs, tc.mode, tc.dest, tc.overwriteSet, tc.jobs); got != tc.wantCode {
				t.Errorf("validateArgs(...) = %d, want %d", got, tc.wantCode)
			}
		})
	}
}
