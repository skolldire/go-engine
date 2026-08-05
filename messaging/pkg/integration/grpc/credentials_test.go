package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTransportCredentials_NilOrDisabledIsInsecure(t *testing.T) {
	creds, err := buildTransportCredentials(nil)
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "insecure", creds.Info().SecurityProtocol)

	creds, err = buildTransportCredentials(&TLSConfig{Enabled: false})
	require.NoError(t, err)
	assert.Equal(t, "insecure", creds.Info().SecurityProtocol)
}

func TestBuildTransportCredentials_EnabledIsTLS(t *testing.T) {
	creds, err := buildTransportCredentials(&TLSConfig{Enabled: true})
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "tls", creds.Info().SecurityProtocol)
}

func TestBuildTransportCredentials_BadCAFile(t *testing.T) {
	_, err := buildTransportCredentials(&TLSConfig{Enabled: true, CAFile: "/does/not/exist.pem"})
	assert.Error(t, err)
}

func TestBuildTransportCredentials_MutualTLSRequiresBoth(t *testing.T) {
	_, err := buildTransportCredentials(&TLSConfig{Enabled: true, CertFile: "cert.pem"})
	assert.Error(t, err, "cert without key must be rejected")

	_, err = buildTransportCredentials(&TLSConfig{Enabled: true, KeyFile: "key.pem"})
	assert.Error(t, err, "key without cert must be rejected")
}
