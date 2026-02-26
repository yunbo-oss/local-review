package otel

import (
	"context"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// Init 初始化 OpenTelemetry Trace
// 当 OTEL_EXPORTER_OTLP_ENDPOINT 未设置时，使用 noop 降级，不阻塞本地开发
// 环境变量：OTEL_EXPORTER_OTLP_ENDPOINT、OTEL_SERVICE_NAME
func Init() (shutdown func(context.Context) error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		logrus.Info("OpenTelemetry: OTEL_EXPORTER_OTLP_ENDPOINT not set, using noop (disabled)")
		return func(context.Context) error { return nil }
	}

	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "local-review-go"
	}

	ctx := context.Background()

	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(), // 内网 Docker 环境，生产建议 TLS
	)
	if err != nil {
		logrus.Warnf("OpenTelemetry trace exporter init failed: %v, using noop", err)
		return func(context.Context) error { return nil }
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter,
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logrus.Infof("OpenTelemetry: Trace enabled, endpoint=%s, service=%s", endpoint, serviceName)

	return func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}
}
