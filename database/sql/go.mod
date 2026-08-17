module github.com/skolldire/go-engine/database/sql

go 1.25.0

toolchain go1.26.5

// The sub-modules are versioned together and never published as separate
// tags, so every cross-module reference resolves through the checkout.
replace github.com/skolldire/go-engine => ../../

require (
	github.com/skolldire/go-engine v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
	gorm.io/gorm v1.31.1
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/sony/gobreaker v1.0.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.39.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
