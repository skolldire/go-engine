module github.com/skolldire/go-engine/aws

go 1.27.2

// The sub-modules are versioned together and never published as separate
// tags, so every cross-module reference resolves through the checkout.
replace github.com/skolldire/go-engine => ../

require (
	github.com/aws/aws-lambda-go v1.55.1
	github.com/aws/aws-sdk-go-v2 v1.47.2
	github.com/aws/aws-sdk-go-v2/config v1.33.8
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.21.10
	github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager v0.4.15
	github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider v1.75.2
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.70.2
	github.com/aws/aws-sdk-go-v2/service/lambda v1.112.0
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.2
	github.com/aws/aws-sdk-go-v2/service/ses v1.42.3
	github.com/aws/aws-sdk-go-v2/service/sns v1.47.4
	github.com/aws/aws-sdk-go-v2/service/sqs v1.52.3
	github.com/aws/aws-sdk-go-v2/service/ssm v1.79.2
	github.com/aws/smithy-go v1.28.4
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/lestrrat-go/jwx/v2 v2.1.7
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	github.com/skolldire/go-engine v0.30.0
	github.com/stretchr/testify v1.12.1
	go.opentelemetry.io/otel v1.47.0
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.21 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.20.8 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.5 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.5 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.45.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.13.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.10.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.38.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.43.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.51.3 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.15 // indirect
	github.com/go-chi/chi/v5 v5.3.2 // indirect
	github.com/go-chi/cors v1.2.2 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.2 // indirect
	github.com/go-playground/universal-translator v0.18.2 // indirect
	github.com/go-playground/validator/v10 v10.30.5 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/goccy/go-json v0.11.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.31.0 // indirect
	github.com/leodido/go-urn v1.5.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.4 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc v1.0.6 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/sony/gobreaker v1.0.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/spf13/viper v1.21.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.72.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.47.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.47.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.1 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20261005182115-fad411399dd8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261005182115-fad411399dd8 // indirect
	google.golang.org/grpc v1.84.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
