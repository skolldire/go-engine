package router

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"time"
)

type ServerConfig struct {
	Listen            string        `mapstructure:"listen" json:"listen"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout" json:"read_header_timeout"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout" json:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout" json:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout" json:"idle_timeout"`
	ShutdownTimeout   time.Duration `mapstructure:"shutdown_timeout" json:"shutdown_timeout"`
	MaxHeaderBytes    int           `mapstructure:"max_header_bytes" json:"max_header_bytes"`
	TLSCertFile       string        `mapstructure:"tls_cert_file" json:"tls_cert_file"`
	TLSKeyFile        string        `mapstructure:"tls_key_file" json:"tls_key_file"`
}

func DefaultServerConfig() ServerConfig {
	return ServerConfig{ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second, ShutdownTimeout: 15 * time.Second, MaxHeaderBytes: 1 << 20}
}
func validateServer(c ServerConfig) error {
	fields := []struct {
		name    string
		invalid bool
	}{
		{"Listen", c.Listen == ""}, {"ReadHeaderTimeout", c.ReadHeaderTimeout <= 0}, {"ReadTimeout", c.ReadTimeout < 0}, {"WriteTimeout", c.WriteTimeout < 0}, {"IdleTimeout", c.IdleTimeout <= 0}, {"ShutdownTimeout", c.ShutdownTimeout <= 0}, {"MaxHeaderBytes", c.MaxHeaderBytes <= 0}, {"TLSCertFile", (c.TLSCertFile == "") != (c.TLSKeyFile == "")},
	}
	for _, f := range fields {
		if f.invalid {
			return &ConfigError{f.name, "invalid value or incomplete TLS pair"}
		}
	}
	return nil
}
func nilPointer(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	return r.Kind() == reflect.Pointer && r.IsNil()
}
func validateServe(ctx context.Context, c ServerConfig, h http.Handler) error {
	if ctx == nil {
		return fmt.Errorf("%w: ctx", ErrInvalidCall)
	}
	if err := validateServer(c); err != nil {
		return err
	}
	if nilPointer(h) {
		return fmt.Errorf("%w: handler", ErrInvalidCall)
	}
	return nil
}
func Serve(ctx context.Context, c ServerConfig, h http.Handler) error {
	if err := validateServe(ctx, c, h); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return nil
	}
	ln, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return err
	}
	return ServeListener(ctx, c, ln, h)
}
func ServeListener(ctx context.Context, c ServerConfig, ln net.Listener, h http.Handler) error {
	if err := validateServe(ctx, c, h); err != nil {
		return err
	}
	if nilPointer(ln) {
		return fmt.Errorf("%w: listener", ErrInvalidCall)
	}
	if ctx.Err() != nil {
		return ln.Close()
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: c.ReadHeaderTimeout, ReadTimeout: c.ReadTimeout, WriteTimeout: c.WriteTimeout, IdleTimeout: c.IdleTimeout, MaxHeaderBytes: c.MaxHeaderBytes}
	done := make(chan error, 1)
	go func() {
		var err error
		if c.TLSCertFile != "" {
			err = srv.ServeTLS(ln, c.TLSCertFile, c.TLSKeyFile)
		} else {
			err = srv.Serve(ln)
		}
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.ShutdownTimeout)
		defer cancel()
		shutdownErr := srv.Shutdown(shutdownCtx)
		// Shutdown closes the listener even when handlers outlive its deadline.
		// Close terminates remaining connections after an unsuccessful graceful drain.
		if shutdownErr != nil {
			shutdownErr = errors.Join(shutdownErr, srv.Close())
		}
		return errors.Join(shutdownErr, <-done)
	}
}
