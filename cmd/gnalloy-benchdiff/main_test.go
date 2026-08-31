package main

import (
	"strings"
	"testing"
)

func TestResolveBenchmarkSelectionUsesSuiteDefaults(t *testing.T) {
	packages, bench, err := resolveBenchmarkSelection("hotpath", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !containsPackage(packages, "./buffer") || !containsPackage(packages, "./codec/http3") {
		t.Fatalf("packages=%v, want suite packages", packages)
	}
	if !strings.Contains(bench, "BenchmarkPooledAllocatorAcquireRelease") {
		t.Fatalf("bench=%s, want suite benchmark regexp", bench)
	}
}

func TestResolveBenchmarkSelectionKeepsExplicitOverrides(t *testing.T) {
	packages, bench, err := resolveBenchmarkSelection("hotpath", "./buffer,./codec", "BenchmarkFixedLengthFrameDecoder")
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 2 || packages[0] != "./buffer" || packages[1] != "./codec" {
		t.Fatalf("packages=%v, want explicit packages", packages)
	}
	if bench != "BenchmarkFixedLengthFrameDecoder" {
		t.Fatalf("bench=%s, want explicit bench", bench)
	}
}

func TestResolveBenchmarkSelectionRejectsUnknownSuite(t *testing.T) {
	_, _, err := resolveBenchmarkSelection("missing", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func containsPackage(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
