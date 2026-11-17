module example.com/config-yaml-composed

go 1.27.4

require (
	github.com/skolldire/go-engine/config v0.0.0
	github.com/skolldire/go-engine/config/yaml v0.0.0
	github.com/skolldire/go-engine/validation v0.0.0
)

require (
	github.com/gabriel-vasile/mimetype v1.4.16 // indirect
	github.com/go-playground/locales v0.14.2 // indirect
	github.com/go-playground/universal-translator v0.18.2 // indirect
	github.com/go-playground/validator/v10 v10.30.6 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/leodido/go-urn v1.5.0 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)

replace github.com/skolldire/go-engine/config => ../../../../../config

replace github.com/skolldire/go-engine/config/yaml => ../../../../../config/yaml

replace github.com/skolldire/go-engine/validation => ../../../../../validation
