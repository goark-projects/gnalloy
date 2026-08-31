package quic

import "fmt"

func validate0RTTClientConfig(cfg Config) error {
	if !cfg.Enable0RTT {
		return Err0RTTDisabled
	}
	if cfg.TLS == nil {
		return ErrMissingTLSConfig
	}
	if cfg.TLS.SessionTicketsDisabled {
		return fmt.Errorf("%w: session tickets disabled", ErrInvalidTLSConfig)
	}
	if cfg.TLS.ClientSessionCache == nil {
		return ErrMissingSessionCache
	}
	return nil
}

func validate0RTTServerConfig(cfg Config) error {
	if !cfg.Enable0RTT {
		return Err0RTTDisabled
	}
	if cfg.TLS == nil {
		return ErrMissingTLSConfig
	}
	if cfg.TLS.SessionTicketsDisabled {
		return fmt.Errorf("%w: session tickets disabled", ErrInvalidTLSConfig)
	}
	return nil
}
