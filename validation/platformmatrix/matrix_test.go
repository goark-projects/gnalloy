package platformmatrix

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLoadDefaultMatrix(t *testing.T) {
	file, err := os.Open("../../scripts/platform-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	matrix, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix.Targets) < 4 {
		t.Fatalf("targets=%d, want cross-platform matrix", len(matrix.Targets))
	}
	if _, ok := matrix.Target("linux", "amd64"); !ok {
		t.Fatal("missing linux/amd64 target")
	}
	if _, ok := matrix.Target("windows", "amd64"); !ok {
		t.Fatal("missing windows/amd64 target")
	}
}

func TestMatrixRejectsDuplicateTargets(t *testing.T) {
	_, err := Load(strings.NewReader(`{
		"version": 1,
		"targets": [
			{"name": "linux-amd64", "goos": "linux", "goarch": "amd64", "gates": [{"name": "compile", "command": ["go", "test"]}]},
			{"name": "linux-amd64", "goos": "linux", "goarch": "amd64", "gates": [{"name": "compile", "command": ["go", "test"]}]}
		]
	}`))
	if !errors.Is(err, ErrInvalidMatrix) {
		t.Fatalf("err=%v, want ErrInvalidMatrix", err)
	}
}

func TestMatrixRejectsEmptyGateCommand(t *testing.T) {
	_, err := Load(strings.NewReader(`{
		"version": 1,
		"targets": [
			{"name": "linux-amd64", "goos": "linux", "goarch": "amd64", "gates": [{"name": "compile"}]}
		]
	}`))
	if !errors.Is(err, ErrInvalidMatrix) {
		t.Fatalf("err=%v, want ErrInvalidMatrix", err)
	}
}
