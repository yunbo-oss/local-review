package agent

import (
	"context"

	"local-review-go/internal/llm"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "local-review-go/agent"

type runSpanContextKey struct{}

// EnsureRunSpan creates exactly one agent.run span across the logic/harness
// boundary. Tool spans therefore share the incoming HTTP distributed trace.
func EnsureRunSpan(ctx context.Context, maxSteps, maxTools int) (context.Context, trace.Span, bool) {
	if active, _ := ctx.Value(runSpanContextKey{}).(bool); active {
		return ctx, trace.SpanFromContext(ctx), false
	}
	spanCtx, span := StartRunSpan(ctx, maxSteps, maxTools)
	return context.WithValue(spanCtx, runSpanContextKey{}, true), span, true
}

// TraceIDFromContext returns the W3C/OpenTelemetry trace id when tracing is
// active. Callers can fall back to a local correlation id under a noop SDK.
func TraceIDFromContext(ctx context.Context) string {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return ""
	}
	return spanContext.TraceID().String()
}

func SpanIDFromContext(ctx context.Context) string {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return ""
	}
	return spanContext.SpanID().String()
}

func SetRunSpanAttributes(span trace.Span, runID, runtimeVersion string) {
	if span == nil {
		return
	}
	span.SetAttributes(
		attribute.String("agent.run_id", runID),
		attribute.String("agent.runtime.version", runtimeVersion),
	)
}

func StartControllerSpan(ctx context.Context, turn int) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, "agent.controller",
		trace.WithAttributes(attribute.Int("agent.turn", turn)),
	)
}

func FinishControllerSpan(span trace.Span, decision AgentDecision, usage llm.TokenUsage, err error) {
	if span == nil {
		return
	}
	span.SetAttributes(
		attribute.String("agent.decision.type", string(decision.Type)),
		attribute.String("agent.decision.reason_code", decision.ReasonCode),
		attribute.Int("agent.action.count", len(decision.Actions)),
		attribute.Int("llm.usage.prompt_tokens", usage.PromptTokens),
		attribute.Int("llm.usage.completion_tokens", usage.CompletionTokens),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "controller_error")
	}
	span.End()
}

func StartActionSpan(ctx context.Context, action AgentAction, turn int) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, "agent.action",
		trace.WithAttributes(
			attribute.String("agent.action.id", action.ID),
			attribute.String("agent.tool.name", action.Tool),
			attribute.Int("agent.turn", turn),
		),
	)
}

func FinishActionSpan(span trace.Span, attempts []ToolResult) {
	if span == nil {
		return
	}
	span.SetAttributes(attribute.Int("agent.action.attempts", len(attempts)))
	if len(attempts) > 0 {
		result := attempts[len(attempts)-1]
		span.SetAttributes(
			attribute.String("agent.action.status", string(result.Status)),
			attribute.String("agent.action.error_code", result.ErrorCode),
			attribute.Int("agent.action.result_count", result.ResultCount),
		)
		if result.Status != ActionSucceeded {
			span.SetStatus(codes.Error, result.ErrorCode)
		}
	}
	span.End()
}

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
