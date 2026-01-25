package routing

import "errors"

var (
	ErrNoBackends           = errors.New("no backends")
	ErrUnknownStrategy      = errors.New("unknown strategy")
	ErrInvalidWeightedPool  = errors.New("invalid weighted pool")
	ErrDiscovery            = errors.New("discovery error")
	ErrDiscoveryNotSet      = errors.New("discovery resolver not set")
	ErrInvalidDiscoveryMode = errors.New("invalid discovery mode")
)
