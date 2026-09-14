module github.com/sfuruya0612/thief/backend

go 1.26.0

// mise.toml の [tools].go と同一バージョンに揃える (AGENTS.md「backend ビルドと CI」参照)
toolchain go1.26.6

require (
	cloud.google.com/go/bigquery v1.83.0
	cloud.google.com/go/logging v1.19.1
	cloud.google.com/go/run v1.22.0
	cloud.google.com/go/storage v1.67.1
	github.com/DataDog/datadog-api-client-go/v2 v2.65.0
	github.com/aws/aws-sdk-go-v2 v1.47.0
	github.com/aws/aws-sdk-go-v2/config v1.33.4
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.21.4
	github.com/aws/aws-sdk-go-v2/service/apigateway v1.47.0
	github.com/aws/aws-sdk-go-v2/service/apigatewayv2 v1.42.0
	github.com/aws/aws-sdk-go-v2/service/athena v1.66.0
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.81.0
	github.com/aws/aws-sdk-go-v2/service/cloudfront v1.73.0
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.87.0
	github.com/aws/aws-sdk-go-v2/service/costexplorer v1.72.0
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.68.0
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.332.0
	github.com/aws/aws-sdk-go-v2/service/ecr v1.65.0
	github.com/aws/aws-sdk-go-v2/service/ecs v1.98.0
	github.com/aws/aws-sdk-go-v2/service/elasticache v1.61.0
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.63.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.64.0
	github.com/aws/aws-sdk-go-v2/service/kinesis v1.54.0
	github.com/aws/aws-sdk-go-v2/service/lambda v1.108.0
	github.com/aws/aws-sdk-go-v2/service/pricing v1.49.0
	github.com/aws/aws-sdk-go-v2/service/rds v1.129.0
	github.com/aws/aws-sdk-go-v2/service/s3 v1.113.1
	github.com/aws/aws-sdk-go-v2/service/savingsplans v1.40.0
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.49.0
	github.com/aws/aws-sdk-go-v2/service/sqs v1.52.0
	github.com/aws/aws-sdk-go-v2/service/ssm v1.78.0
	github.com/aws/aws-sdk-go-v2/service/sso v1.38.0
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.43.0
	github.com/aws/aws-sdk-go-v2/service/sts v1.50.0
	github.com/aws/aws-sdk-go-v2/service/wafv2 v1.83.0
	github.com/aws/smithy-go v1.28.1
	github.com/charmbracelet/bubbletea v1.3.10
	github.com/coder/websocket v1.8.15
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/spf13/cobra v1.10.2
	golang.org/x/sync v0.23.0
	google.golang.org/api v0.297.0
	google.golang.org/genproto v0.0.0-20260911204522-f61a6ca850bd
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260911204522-f61a6ca850bd
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.12
	gopkg.in/yaml.v3 v3.0.1
)

require (
	cel.dev/expr v0.25.3 // indirect
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.23.2 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/iam v1.13.0 // indirect
	cloud.google.com/go/longrunning v1.2.0 // indirect
	cloud.google.com/go/monitoring v1.30.0 // indirect
	github.com/DataDog/zstd v1.5.7 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.38.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.62.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.62.0 // indirect
	github.com/apache/arrow/go/v15 v15.0.2 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.20 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.20.4 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.41.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.13.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.10.0 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/lipgloss v1.1.0 // indirect
	github.com/charmbracelet/x/ansi v0.11.8 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.39.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.3.3 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.5 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.22 // indirect
	github.com/googleapis/gax-go/v2 v2.24.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.20.0 // indirect
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.30 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.29 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/spiffe/go-spiffe/v2 v2.8.1 // indirect
	github.com/xo/terminfo v1.2.0 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.46.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.71.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.71.0 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/sdk v1.46.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/exp v0.0.0-20260908205506-85c1c2202aba // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/oauth2 v0.37.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/telemetry v0.0.0-20260910141331-15ceca2b0a1f // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	golang.org/x/tools v0.50.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260911204522-f61a6ca850bd // indirect
)
