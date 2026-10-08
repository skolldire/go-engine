package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// TLSConfig configures transport security for the gRPC client. When Enabled is
// false the client uses an insecure (plaintext) connection, which is only
// appropriate for local development or a trusted network.
type TLSConfig struct {
	Enabled bool `mapstructure:"enabled" json:"enabled"`
	// CAFile is an optional PEM bundle used to verify the server certificate.
	// When empty, the host's root CAs are used.
	CAFile string `mapstructure:"ca_file" json:"ca_file"`
	// CertFile and KeyFile enable mutual TLS (client certificate).
	CertFile string `mapstructure:"cert_file" json:"cert_file"`
	KeyFile  string `mapstructure:"key_file" json:"key_file"`
	// ServerName overrides the name checked against the server certificate.
	ServerName string `mapstructure:"server_name" json:"server_name"`
	// InsecureSkipVerify disables server certificate verification. It must only
	// be used in tests; it defeats the purpose of TLS in production.
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify" json:"insecure_skip_verify"`
}

// buildTransportCredentials returns the gRPC transport credentials for cfg.
// When cfg is nil or disabled, insecure credentials are returned.
func buildTransportCredentials(cfg *TLSConfig) (credentials.TransportCredentials, error) {
	if cfg == nil || !cfg.Enabled {
		return insecure.NewCredentials(), nil
	}

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // opt-in, documented for test use only
	}

	if cfg.CAFile != "" {
		caPEM, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read gRPC CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("no valid certificates found in gRPC CA file %q", cfg.CAFile)
		}
		tlsCfg.RootCAs = pool
	}

	if cfg.CertFile != "" || cfg.KeyFile != "" {
		if cfg.CertFile == "" || cfg.KeyFile == "" {
			return nil, fmt.Errorf("both cert_file and key_file are required for gRPC mutual TLS")
		}
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load gRPC client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return credentials.NewTLS(tlsCfg), nil
}
