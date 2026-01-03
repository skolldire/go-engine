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

