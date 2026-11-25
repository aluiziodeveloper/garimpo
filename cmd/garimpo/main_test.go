package main

import (
	"bytes"
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
