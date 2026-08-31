package microbench

import (
	"strings"
	"testing"
)

func TestLookupHotPathSuiteBuildsStableBenchdiffInputs(t *testing.T) {
	suite, ok := Lookup("hotpath")
	if !ok {
		t.Fatal("missing hotpath suite")
	}
	packages := suite.Packages()
	for _, want := range []string{"./buffer", "./channel", "./codec", "./codec/http3"} {
		if !contains(packages, want) {
			t.Fatalf("packages=%v, want %s", packages, want)
		}
	}
	bench := suite.BenchmarkRegexp()
	for _, want := range []string{"BenchmarkPooledAllocatorAcquireRelease", "BenchmarkPipelineInboundNoop", "BenchmarkHeaderDecoderFragmentedBlock"} {
		if !strings.Contains(bench, want) {
			t.Fatalf("bench=%s, want %s", bench, want)
		}
	}
	if !strings.HasPrefix(bench, "^(") || !strings.HasSuffix(bench, ")$") {
		t.Fatalf("bench=%s, want anchored regexp", bench)
	}
}

func TestLookupRejectsUnknownSuite(t *testing.T) {
	if _, ok := Lookup("missing"); ok {
		t.Fatal("unexpected suite")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
