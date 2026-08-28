package benchdiff

import "errors"

var (
	ErrInvalidRunner          = errors.New("gnalloy/benchmarks/benchdiff: invalid runner")
	ErrInvalidReport          = errors.New("gnalloy/benchmarks/benchdiff: invalid report")
	ErrNoComparableBenchmarks = errors.New("gnalloy/benchmarks/benchdiff: no comparable benchmarks")
)
