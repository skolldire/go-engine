module example.com/config-yaml-minimal

go 1.27.4

require (
	github.com/skolldire/go-engine/config v0.0.0
	github.com/skolldire/go-engine/config/yaml v0.0.0
)

require (
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
)

replace github.com/skolldire/go-engine/config => ../../../../../config

replace github.com/skolldire/go-engine/config/yaml => ../../../../../config/yaml
