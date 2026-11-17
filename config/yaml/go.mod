module github.com/skolldire/go-engine/config/yaml

go 1.27.4

require (
	github.com/skolldire/go-engine/config v0.0.0
	go.yaml.in/yaml/v3 v3.0.6
)

require github.com/go-viper/mapstructure/v2 v2.5.0 // indirect

replace github.com/skolldire/go-engine/config => ..
