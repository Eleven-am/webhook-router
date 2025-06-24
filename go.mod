module webhook-router

go 1.24

require (
	cloud.google.com/go/pubsub v1.49.0
	github.com/alicebob/miniredis/v2 v2.35.0
	github.com/aws/aws-sdk-go-v2 v1.36.5
	github.com/aws/aws-sdk-go-v2/config v1.29.17
	github.com/aws/aws-sdk-go-v2/credentials v1.17.70
	github.com/aws/aws-sdk-go-v2/service/sns v1.34.7
	github.com/aws/aws-sdk-go-v2/service/sqs v1.38.8
	github.com/confluentinc/confluent-kafka-go v1.9.2
	github.com/emersion/go-ical v0.0.0-20250609112844-439c63cef608
	github.com/emersion/go-imap v1.2.1
	github.com/emersion/go-message v0.18.2
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6
	github.com/emersion/go-vcard v0.0.0-20241024213814-c9703dde27ff
	github.com/expr-lang/expr v1.17.5
	github.com/go-playground/validator/v10 v10.26.0
	github.com/go-redis/redis/v8 v8.11.5
	github.com/go-redsync/redsync/v4 v4.13.0
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/grokify/html-strip-tags-go v0.1.0
	github.com/heimdalr/dag v1.5.0
	github.com/jackc/pgx/v5 v5.7.5
	github.com/joho/godotenv v1.5.1
	github.com/lucsky/cuid v1.2.1
	github.com/mattn/go-sqlite3 v1.14.28
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/robfig/cron/v3 v3.0.1
	github.com/samber/lo v1.51.0
	github.com/sony/gobreaker v1.0.0
	github.com/streadway/amqp v1.1.0
	github.com/stretchr/testify v1.10.0
	github.com/swaggo/http-swagger v1.3.4
	github.com/swaggo/swag v1.16.4
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.39.0
	golang.org/x/sync v0.15.0
	golang.org/x/time v0.12.0
	google.golang.org/api v0.238.0
)

require (
	cloud.google.com/go v0.121.2 // indirect
	cloud.google.com/go/auth v0.16.2 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.7.0 // indirect
	cloud.google.com/go/iam v1.5.2 // indirect
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.34.0 // indirect
	github.com/aws/smithy-go v1.22.4 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.21.1 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/spec v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.1 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/googleapis/gax-go/v2 v2.14.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/swaggo/files v1.0.1 // indirect
	github.com/teambition/rrule-go v1.8.2 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.61.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/otel v1.36.0 // indirect
	go.opentelemetry.io/otel/metric v1.36.0 // indirect
	go.opentelemetry.io/otel/trace v1.36.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	golang.org/x/tools v0.34.0 // indirect
	google.golang.org/genproto v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/grpc v1.73.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
