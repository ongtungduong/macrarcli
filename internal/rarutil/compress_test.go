package rarutil

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

// TestEntryMethod maps Options to the ZIP compression method.
func TestEntryMethod(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want uint16
	}{
		{"default deflate", Options{Level: DefaultLevel}, zip.Deflate},
		{"explicit level keeps deflate", Options{Level: 9}, zip.Deflate},
		{"store", Options{Store: true}, zip.Store},
		{"store overrides level", Options{Store: true, Level: 9}, zip.Store},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := entryMethod(tc.opts); got != tc.want {
				t.Errorf("entryMethod(%+v) = %d, want %d", tc.opts, got, tc.want)
			}
		})
	}
}

// TestRegisterCompressor_LevelRoundTrips writes a payload through a Deflate
// compressor registered at an explicit level and confirms the bytes survive.
func TestRegisterCompressor_LevelRoundTrips(t *testing.T) {
	for _, level := range []int{0, 1, 6, 9} {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		registerCompressor(zw, Options{Level: level})

		w, err := zw.CreateHeader(&zip.FileHeader{Name: "a.txt", Method: zip.Deflate})
		if err != nil {
			t.Fatalf("level %d: CreateHeader: %v", level, err)
		}
		payload := bytes.Repeat([]byte("rar2zip "), 256)
		if _, err := w.Write(payload); err != nil {
			t.Fatalf("level %d: write: %v", level, err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("level %d: close: %v", level, err)
		}

		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatalf("level %d: open: %v", level, err)
		}
		rc, err := zr.File[0].Open()
		if err != nil {
			t.Fatalf("level %d: entry open: %v", level, err)
		}
		got, _ := io.ReadAll(rc)
		rc.Close()
		if !bytes.Equal(got, payload) {
			t.Errorf("level %d: round-trip mismatch", level)
		}
		if zr.File[0].Method != zip.Deflate {
			t.Errorf("level %d: method = %d, want Deflate", level, zr.File[0].Method)
		}
	}
}
