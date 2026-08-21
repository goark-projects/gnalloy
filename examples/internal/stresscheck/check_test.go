package stresscheck

import (
	"runtime"
	"testing"
	"time"

	"goark.dev/gnalloy/examples/internal/stressclient"
)

func TestRunChecksRawAndLengthFieldLeaks(t *testing.T) {
	skipUnsupportedNativeTCP(t)

	result, err := Run(nil, Config{
		Protocol:        ProtocolBoth,
		Scenario:        stressclient.ScenarioShort,
		Connections:     2,
		MessagesPerConn: 2,
		PayloadSize:     16,
		Timeout:         3 * time.Second,
		DrainTimeout:    3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Errors != 0 || result.Requests != 8 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunRejectsInvalidProtocol(t *testing.T) {
	_, err := Run(nil, Config{
		Protocol:        "bad",
		Scenario:        stressclient.ScenarioShort,
		Connections:     1,
		MessagesPerConn: 1,
		PayloadSize:     1,
	})
	if err != stressclient.ErrInvalidProtocol {
		t.Fatalf("err=%v, want %v", err, stressclient.ErrInvalidProtocol)
	}
}

func skipUnsupportedNativeTCP(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "netbsd", "openbsd", "dragonfly", "windows":
	default:
		t.Skipf("native tcp is unsupported on %s", runtime.GOOS)
	}
}
