package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"goark.dev/gnalloy/benchmarks/parity"
)

func TestRunStrictExternalRejectsSkippedHarness(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "baseline.json")
	config := `{
		"name": "strict",
		"scenarios": [{
			"name": "netty tcp echo",
			"framework": "netty",
			"protocol": "tcp-echo",
			"skip": true,
			"skipReason": "external harness missing"
		}]
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	err := run(configPath, "", parity.FormatJSON, true, true, 0)
	if !errors.Is(err, parity.ErrExternalHarness) {
		t.Fatalf("err=%v, want ErrExternalHarness", err)
	}
}

func TestRunDryRunKeepsDefaultExternalBehavior(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "baseline.json")
	outPath := filepath.Join(t.TempDir(), "report.json")
	config := `{
		"name": "dry",
		"scenarios": [{
			"name": "netty tcp echo",
			"framework": "netty",
			"protocol": "tcp-echo",
			"skip": true,
			"skipReason": "external harness missing"
		}]
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(configPath, outPath, parity.FormatJSON, true, false, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatal(err)
	}
}
