package app

import (
	"context"
	"fmt"

	grpcClient "github.com/skolldire/go-engine/pkg/clients/grpc"
	"github.com/skolldire/go-engine/pkg/clients/rest"
	"github.com/skolldire/go-engine/pkg/config/viper"
	"github.com/skolldire/go-engine/pkg/core/registry"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// RegisterDefaultClients registers default client factories ("rest" and "grpc_client")
// in the global registry.
//
// The "rest" factory expects a rest.Config and returns a REST client created via rest.NewClient.
// The "grpc_client" factory expects a grpcClient.Config and returns a gRPC client created via grpcClient.NewCliente.
// Returns an error if a registration fails or if a factory receives an invalid configuration type.
func RegisterDefaultClients(log logger.Service) error {
	reg := registry.GetRegistry(log)

	if err := reg.Register("rest", func(ctx context.Context, cfg interface{}, log logger.Service) (interface{}, error) {
		restConfig, ok := cfg.(rest.Config)
		if !ok {
			return nil, fmt.Errorf("invalid configuration for REST client")
		}
		return rest.NewClient(restConfig, log), nil
	}); err != nil {
		return err
	}

	if err := reg.Register("grpc_client", func(ctx context.Context, cfg interface{}, log logger.Service) (interface{}, error) {
		grpcConfig, ok := cfg.(grpcClient.Config)
		if !ok {
			return nil, fmt.Errorf("invalid configuration for gRPC client")
		}
		return grpcClient.NewCliente(grpcConfig, log)
	}); err != nil {
		return err
	}

	return nil
}

func (i *clients) initializeWithRegistry(conf *viper.Config) {
}
