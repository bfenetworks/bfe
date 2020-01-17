package bfe_discovery

import "time"

type Config struct {
	Addrs []string
	DialTimeout time.Duration

	// TODO bfe_ prefix
	PathPrefix string

	// TODO cancel context timeout
	OpTimeout time.Duration

	// TODO tls
	TLSConfig *TLSConfig

	// TODO auth
	Username string
	Password string
	Token string
}

type TLSConfig struct {
	CertFile string
	KeyFile string
	CACertFile string
}