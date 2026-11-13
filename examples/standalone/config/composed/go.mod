module example.com/config-composed

go 1.27.4

require (
	github.com/skolldire/go-engine/concurrency v0.0.0
	github.com/skolldire/go-engine/config v0.0.0
	github.com/skolldire/go-engine/config/viper v0.0.0
)

require (
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.1.0 // indirect
	github.com/spf13/viper v1.21.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)

replace github.com/skolldire/go-engine/config => ../../../../config

replace github.com/skolldire/go-engine/concurrency => ../../../../concurrency

replace github.com/skolldire/go-engine/config/viper => ../../../../config/viper
