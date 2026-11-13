module example.com/config-minimal

go 1.27.4

require github.com/skolldire/go-engine/config v0.0.0

replace github.com/skolldire/go-engine/config => ../../../../config

require github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
