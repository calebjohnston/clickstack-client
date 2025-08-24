package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	serviceName    = "otel-demo-service"
	serviceVersion = "1.0.0"
	// Default OpenTelemetry collector endpoint
	otelCollectorEndpoint = "localhost:4317"
)

func main() {
	// Get collector endpoint from environment variable or use default
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = otelCollectorEndpoint
	}

	// Initialize OpenTelemetry
	ctx := context.Background()
	
	// Setup resource
	res := setupResource()

	// Setup trace provider
	traceProvider, err := setupTraceProvider(ctx, endpoint, res)
	if err != nil {
		log.Fatalf("Failed to setup trace provider: %v", err)
	}
	defer func() {
		if err := traceProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down trace provider: %v", err)
		}
	}()

	// Setup log provider
	logProvider, err := setupLogProvider(ctx, endpoint, res)
	if err != nil {
		log.Fatalf("Failed to setup log provider: %v", err)
	}
	defer func() {
		if err := logProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down log provider: %v", err)
		}
	}()

	// Setup metric provider
	metricProvider, err := setupMetricProvider(ctx, endpoint, res)
	if err != nil {
		log.Fatalf("Failed to setup metric provider: %v", err)
	}
	defer func() {
		if err := metricProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down metric provider: %v", err)
		}
	}()

	// Set global providers
	otel.SetTracerProvider(traceProvider)
	global.SetLoggerProvider(logProvider)
	otel.SetMeterProvider(metricProvider)

	// Get tracer, logger, and meter
	tracer := otel.Tracer(serviceName)
	logger := global.GetLoggerProvider().Logger(serviceName)
	meter := otel.Meter(serviceName)

	// Demonstrate tracing, logging, and metrics
	fmt.Println("Starting OpenTelemetry demo...")
	
	// Create metrics
	requestCounter, err := meter.Int64Counter(
		"requests_total",
		metric.WithDescription("Total number of requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create counter: %v", err)
	}

	requestDuration, err := meter.Float64Histogram(
		"request_duration_seconds",
		metric.WithDescription("Duration of requests"),
		metric.WithUnit("s"),
	)
	if err != nil {
		log.Fatalf("Failed to create histogram: %v", err)
	}

	activeConnections, err := meter.Int64UpDownCounter(
		"active_connections",
		metric.WithDescription("Number of active connections"),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create up-down counter: %v", err)
	}

	// Create a gauge callback for memory usage
	_, err = meter.Int64ObservableGauge(
		"memory_usage_bytes",
		metric.WithDescription("Current memory usage"),
		metric.WithUnit("By"),
		metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
			// Simulate memory usage
			memUsage := int64(1024 * 1024 * (50 + rand.Intn(50))) // 50-100 MB
			observer.Observe(memUsage, metric.WithAttributes(
				attribute.String("memory_type", "heap"),
			))
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create gauge: %v", err)
	}
	
	// Create a root span
	ctx, rootSpan := tracer.Start(ctx, "main-operation",
		trace.WithAttributes(
			attribute.String("operation.type", "demo"),
			attribute.String("user.id", "12345"),
		))
	defer rootSpan.End()

	// Log at the start of the operation
	logRecord(ctx, logger, "Starting main operation", otellog.SeverityInfo,
		otellog.String("component", "main"),
		otellog.String("operation", "start"))

	// Simulate some work with nested spans and metrics
	if err := simulateWork(ctx, tracer, logger, requestCounter, requestDuration, activeConnections); err != nil {
		rootSpan.SetStatus(codes.Error, err.Error())
		
		// Log the error
		logRecord(ctx, logger, fmt.Sprintf("Operation failed: %v", err), otellog.SeverityError,
			otellog.String("component", "main"),
			otellog.String("error", err.Error()))
	} else {
		rootSpan.SetStatus(codes.Ok, "Operation completed successfully")
		
		// Log success
		logRecord(ctx, logger, "Operation completed successfully", otellog.SeverityInfo,
			otellog.String("component", "main"),
			otellog.String("operation", "complete"))
	}

	fmt.Println("Demo completed. Check your OpenTelemetry collector for traces, logs, and metrics!")

	// Give some time for exports to complete
	time.Sleep(5 * time.Second)
}

// Helper function to create and emit log records
func logRecord(ctx context.Context, logger otellog.Logger, message string, severity otellog.Severity, attrs ...otellog.KeyValue) {
	var record otellog.Record
	record.SetTimestamp(time.Now())
	record.SetBody(otellog.StringValue(message))
	record.SetSeverity(severity)
	record.AddAttributes(attrs...)
	logger.Emit(ctx, record)
}

func setupResource() *resource.Resource {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(serviceVersion),
		semconv.ServiceInstanceID("instance-1"),
		attribute.String("environment", "development"),
	)
	return res
}

func setupTraceProvider(ctx context.Context, endpoint string, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	// Create OTLP trace exporter
	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create trace provider
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	return traceProvider, nil
}

func setupLogProvider(ctx context.Context, endpoint string, res *resource.Resource) (*sdklog.LoggerProvider, error) {
	// Create OTLP log exporter
	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	logExporter, err := otlploggrpc.New(ctx, otlploggrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create log exporter: %w", err)
	}

	// Create log provider
	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)

	return logProvider, nil
}

func setupMetricProvider(ctx context.Context, endpoint string, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	// Create OTLP metric exporter
	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	// Create metric provider
	metricProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(10*time.Second))), // Export every 10 seconds
		sdkmetric.WithResource(res),
	)

	return metricProvider, nil
}

func simulateWork(ctx context.Context, tracer trace.Tracer, logger otellog.Logger, 
	requestCounter metric.Int64Counter, requestDuration metric.Float64Histogram, 
	activeConnections metric.Int64UpDownCounter) error {
	
	// Increment active connections
	activeConnections.Add(ctx, 1, metric.WithAttributes(
		attribute.String("connection_type", "database"),
	))
	defer activeConnections.Add(ctx, -1, metric.WithAttributes(
		attribute.String("connection_type", "database"),
	))

	// Record request start
	requestStart := time.Now()
	requestCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("method", "GET"),
		attribute.String("endpoint", "/api/users"),
		attribute.String("status", "processing"),
	))
	// Create a child span for database operation
	ctx, dbSpan := tracer.Start(ctx, "database-query",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.name", "userdb"),
			attribute.String("db.operation", "SELECT"),
		))
	defer dbSpan.End()

	// Log database query start
	logRecord(ctx, logger, "Executing database query", otellog.SeverityDebug,
		otellog.String("component", "database"),
		otellog.String("query", "SELECT * FROM users WHERE id = ?"))

	// Simulate database work
	dbDuration := time.Duration(80+rand.Intn(40)) * time.Millisecond
	time.Sleep(dbDuration)

	// Record database metrics
	requestDuration.Record(ctx, dbDuration.Seconds(), metric.WithAttributes(
		attribute.String("operation", "database_query"),
		attribute.String("db.system", "postgresql"),
	))

	// Add some attributes to the span
	dbSpan.SetAttributes(
		attribute.Int("db.rows_affected", 1),
		attribute.String("db.query_time", fmt.Sprintf("%.0fms", dbDuration.Seconds()*1000)),
	)

	// Increment active connections for API call
	activeConnections.Add(ctx, 1, metric.WithAttributes(
		attribute.String("connection_type", "http_client"),
	))
	defer activeConnections.Add(ctx, -1, metric.WithAttributes(
		attribute.String("connection_type", "http_client"),
	))

	// Create another child span for API call
	ctx, apiSpan := tracer.Start(ctx, "external-api-call",
		trace.WithAttributes(
			attribute.String("http.method", "GET"),
			attribute.String("http.url", "https://api.example.com/data"),
		))
	defer apiSpan.End()

	// Log API call
	logRecord(ctx, logger, "Making external API call", otellog.SeverityInfo,
		otellog.String("component", "api-client"),
		otellog.String("url", "https://api.example.com/data"),
		otellog.String("method", "GET"))

	// Simulate API call
	apiDuration := time.Duration(150+rand.Intn(100)) * time.Millisecond
	time.Sleep(apiDuration)

	// Record API metrics
	requestDuration.Record(ctx, apiDuration.Seconds(), metric.WithAttributes(
		attribute.String("operation", "api_call"),
		attribute.String("http.method", "GET"),
		attribute.Int("http.status_code", 200),
	))

	// Simulate success
	apiSpan.SetAttributes(
		attribute.Int("http.status_code", 200),
		attribute.String("http.response_time", fmt.Sprintf("%.0fms", apiDuration.Seconds()*1000)),
	)

	// Log successful API response
	logRecord(ctx, logger, "API call completed successfully", otellog.SeverityInfo,
		otellog.String("component", "api-client"),
		otellog.Int("status_code", 200),
		otellog.String("response_time", fmt.Sprintf("%.0fms", apiDuration.Seconds()*1000)))

	// Record final request metrics
	totalDuration := time.Since(requestStart)
	requestDuration.Record(ctx, totalDuration.Seconds(), metric.WithAttributes(
		attribute.String("operation", "total_request"),
		attribute.String("method", "GET"),
		attribute.String("endpoint", "/api/users"),
		attribute.String("status", "success"),
	))

	// Update request counter with final status
	requestCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("method", "GET"),
		attribute.String("endpoint", "/api/users"),
		attribute.String("status", "success"),
	))

	return nil
}