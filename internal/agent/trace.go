package agent

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "local-review-go/agent"

// StartRunSpan agent.run（不记录 question/profile 正文）
func StartRunSpan(ctx context.Context, maxSteps, maxTools int) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, "agent.run",
		trace.WithAttributes(
			attribute.Int("agent.max_steps", maxSteps),
			attribute.Int("agent.max_tool_calls", maxTools),
		),
	)
}

// StartToolSpan tool.execute
func StartToolSpan(ctx context.Context, toolName string) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, "tool.execute",
		trace.WithAttributes(attribute.String("tool.name", toolName)),
	)
}
