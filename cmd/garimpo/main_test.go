package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantOut string
		wantErr string
	}{
		{name: "version", args: []string{"version"}, wantOut: "garimpo dev (" + runtime.Version() + ")\n"},
		{name: "no command", args: nil, wantErr: "usage: garimpo <command>"},
		{name: "unknown command", args: []string{"dig"}, wantErr: `unknown command "dig"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(tt.args, &out)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("run(%q) error = %v, want it to contain %q", tt.args, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("run(%q) unexpected error: %v", tt.args, err)
			}
			if got := out.String(); got != tt.wantOut {
				t.Errorf("run(%q) output = %q, want %q", tt.args, got, tt.wantOut)
			}
		})
	}
}

func TestScanCountsTheLibrarySources(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{
		"bytes/bytes.go":      "package bytes\n",
		"bytes/bytes_test.go": "package bytes\n",
		"strings/strings.go":  "package strings\n\nfunc F() {}\n",
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := run([]string{"scan", "-corpus", dir}, &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "documents=2 bytes=43 chunks=2 average=21 largest=29\n"; got != want {
		t.Errorf("scan output = %q, want %q", got, want)
	}
}

func TestScanRejectsAZeroWindow(t *testing.T) {
	err := run([]string{"scan", "-window", "0"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid window") {
		t.Errorf("scan -window 0: error = %v", err)
	}
}

func TestScanReportsAMissingCorpus(t *testing.T) {
	err := run([]string{"scan", "-corpus", filepath.Join(t.TempDir(), "nope")}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no such file or directory") {
		t.Errorf("scan of a missing corpus: error = %v", err)
	}
}
