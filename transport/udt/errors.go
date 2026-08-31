package udt

import "errors"

var ErrUnsupportedUDT = errors.New("gnalloy/transport/udt: udt transport is unsupported without a driver")
