package rxtx

import "errors"

var ErrUnsupportedRXTX = errors.New("gnalloy/transport/rxtx: rxtx transport is unsupported without a driver")
