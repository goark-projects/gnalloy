package parity

import "errors"

var (
	ErrInvalidSpec     = errors.New("gnalloy/benchmarks/parity: invalid spec")
	ErrInvalidScenario = errors.New("gnalloy/benchmarks/parity: invalid scenario")
	ErrInvalidFormat   = errors.New("gnalloy/benchmarks/parity: invalid report format")
)
