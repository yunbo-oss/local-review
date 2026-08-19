package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"local-review-go/internal/llm"
)

type RuntimeEventType string

const (
	RuntimeEventStarted    RuntimeEventType = "run_started"
	RuntimeEventDecision   RuntimeEventType = "decision"
	RuntimeEventBatch      RuntimeEventType = "action_batch"
	RuntimeEventCheckpoint RuntimeEventType = "checkpoint"
	RuntimeEventCompleted  RuntimeEventType = "run_completed"
)

type RuntimeEvent struct {
	Type       RuntimeEventType `json:"type"`
	RunID      string           `json:"run_id"`
	Revision   int64            `json:"revision"`
	Decision   *AgentDecision   `json:"decision,omitempty"`
	Batch      *BatchExecution  `json:"batch,omitempty"`
	Status     RuntimeStatus    `json:"status,omitempty"`
	StopReason string           `json:"stop_reason,omitempty"`
}

type RuntimeEventCallback func(RuntimeEvent)

type ReactRuntimeConfig struct {
	RunTimeout            time.Duration
	ControllerOutputRunes int
	OnEvent               RuntimeEventCallback
}

func DefaultReactRuntimeConfig() ReactRuntimeConfig {
	return ReactRuntimeConfig{RunTimeout: 30 * time.Second, ControllerOutputRunes: 1200}
}

type ReactRunResult struct {
	State         *AgentState      `json:"state"`
	Decisions     []AgentDecision  `json:"decisions"`
	Batches       []BatchExecution `json:"batches"`
	Clarification string           `json:"clarification,omitempty"`
	Usage         llm.TokenUsage   `json:"usage"`
	ModelCalls    int              `json:"model_calls"`
	Err           error            `json:"-"`
}

type ReactRuntime struct {
	Controller   DecisionController
	Executor     *ParallelActionExecutor
	GapEvaluator EvidenceGapEvaluator
	Checkpointer AgentCheckpointer
	Config       ReactRuntimeConfig
}

func (r *ReactRuntime) Run(ctx context.Context, state *AgentState) ReactRunResult {
	result := ReactRunResult{State: state}
	if r == nil || r.Controller == nil || r.Executor == nil {
		result.Err = fmt.Errorf("react runtime is not configured")
		return result
	}
	if err := state.Validate(); err != nil {
		result.Err = err
		return result
	}
	switch state.Status {
	case RuntimeCompleted, RuntimeNeedsClarify, RuntimeFailed, RuntimeCancelled, RuntimeBudgetExhausted:
		return result
	}
	cfg := r.Config
	if cfg.RunTimeout <= 0 {
		cfg = DefaultReactRuntimeConfig()
	}
	runCtx, cancel := context.WithTimeout(ctx, cfg.RunTimeout)
	defer cancel()
	if state.Status == RuntimeReady {
		state.Status = RuntimeRunning
		state.Revision++
		state.UpdatedAt = time.Now().UnixMilli()
	}
	r.emit(RuntimeEvent{Type: RuntimeEventStarted, RunID: state.RunID, Revision: state.Revision, Status: state.Status})
	if err := r.save(runCtx, state); err != nil {
		result.Err = err
		return result
	}

	for {
		if err := runCtx.Err(); err != nil {
			state.Status = RuntimeCancelled
			state.StopReason = "context_cancelled"
			result.Err = err
			_ = r.save(context.WithoutCancel(ctx), state)
			return result
		}
		if state.Budget.Turns >= state.Budget.MaxTurns {
			state.Status = RuntimeBudgetExhausted
			state.StopReason = ErrMaxSteps
			result.Err = errors.Join(result.Err, r.finish(runCtx, state))
			return result
		}
		if state.Budget.NoNoveltyRounds > 0 && state.Budget.NoNoveltyRounds >= state.Budget.MaxNoNoveltyRounds {
			state.Status = RuntimeBudgetExhausted
			state.StopReason = "no_evidence_novelty"
			result.Err = errors.Join(result.Err, r.finish(runCtx, state))
			return result
		}

		gapEvaluator := r.GapEvaluator
		if gapEvaluator == nil {
			gapEvaluator = DeterministicEvidenceGapEvaluator{}
		}
		state.Gaps = gapEvaluator.Evaluate(state)
		updateCandidateEvidenceScores(state)
		input := ControllerInputFromState(state, cfg.ControllerOutputRunes)
		controllerCtx, controllerSpan := StartControllerSpan(runCtx, state.Budget.Turns+1)
		decision, usage, err := r.Controller.Decide(controllerCtx, input)
		FinishControllerSpan(controllerSpan, decision, usage, err)
		result.ModelCalls++
		result.Usage = addUsage(result.Usage, usage)
		state.Budget.Turns++
		state.Revision++
		state.UpdatedAt = time.Now().UnixMilli()
		if runCtx.Err() != nil {
			state.Status = RuntimeCancelled
			state.StopReason = "context_cancelled"
			result.Err = runCtx.Err()
			result.Err = errors.Join(result.Err, r.finish(context.WithoutCancel(ctx), state))
			return result
		}
		decisionErr := err
		if decisionErr == nil {
			decisionErr = state.ValidateDecision(decision)
		}
		if decisionErr != nil && runCtx.Err() == nil {
			// One bounded repair call makes structured-output validation part of
			// the controller protocol instead of a terminal failure. It is the
			// same logical turn, so it counts model cost but not another ReAct
			// observation/action turn.
			repairInput := input
			repairInput.ValidationFeedback = truncateIntentText(decisionErr.Error(), 240)
			repairCtx, repairSpan := StartControllerSpan(runCtx, state.Budget.Turns)
			repaired, repairUsage, repairErr := r.Controller.Decide(repairCtx, repairInput)
			FinishControllerSpan(repairSpan, repaired, repairUsage, repairErr)
			result.ModelCalls++
			result.Usage = addUsage(result.Usage, repairUsage)
			state.Revision++
			state.UpdatedAt = time.Now().UnixMilli()
			if repairErr == nil {
				repairErr = state.ValidateDecision(repaired)
			}
			if repairErr == nil {
				decision = repaired
				decisionErr = nil
			} else {
				decisionErr = errors.Join(decisionErr, repairErr)
			}
		}
		if decisionErr != nil {
			state.Status = RuntimeFailed
			state.StopReason = "invalid_controller_decision"
			result.Err = decisionErr
			result.Err = errors.Join(result.Err, r.finish(runCtx, state))
			return result
		}
		result.Decisions = append(result.Decisions, decision)
		r.emit(RuntimeEvent{Type: RuntimeEventDecision, RunID: state.RunID, Revision: state.Revision, Decision: &decision, Status: state.Status})

		switch decision.Type {
		case DecisionFinish:
			state.Status = RuntimeCompleted
			state.StopReason = decision.ReasonCode
			result.Err = errors.Join(result.Err, r.finish(runCtx, state))
			return result
		case DecisionClarify:
			state.Status = RuntimeNeedsClarify
			state.StopReason = decision.ReasonCode
			result.Clarification = decision.Clarification
			result.Err = errors.Join(result.Err, r.finish(runCtx, state))
			return result
		case DecisionAct:
			batch, executeErr := r.Executor.Execute(runCtx, state, decision)
			result.Batches = append(result.Batches, batch)
			r.emit(RuntimeEvent{Type: RuntimeEventBatch, RunID: state.RunID, Revision: state.Revision, Batch: &batch, Status: state.Status})
			if executeErr != nil {
				state.Status = RuntimeFailed
				state.StopReason = "executor_error"
				result.Err = executeErr
				result.Err = errors.Join(result.Err, r.finish(runCtx, state))
				return result
			}
			if err := r.save(runCtx, state); err != nil {
				state.Status = RuntimeFailed
				state.StopReason = "checkpoint_error"
				result.Err = err
				return result
			}
		}
	}
}

func (r *ReactRuntime) save(ctx context.Context, state *AgentState) error {
	if r.Checkpointer == nil {
		return nil
	}
	if err := r.Checkpointer.Save(ctx, state); err != nil {
		return fmt.Errorf("save agent checkpoint: %w", err)
	}
	r.emit(RuntimeEvent{Type: RuntimeEventCheckpoint, RunID: state.RunID, Revision: state.Revision, Status: state.Status})
	return nil
}

func (r *ReactRuntime) finish(ctx context.Context, state *AgentState) error {
	state.Revision++
	state.UpdatedAt = time.Now().UnixMilli()
	err := r.save(context.WithoutCancel(ctx), state)
	r.emit(RuntimeEvent{
		Type: RuntimeEventCompleted, RunID: state.RunID, Revision: state.Revision,
		Status: state.Status, StopReason: state.StopReason,
	})
	return err
}

func (r *ReactRuntime) emit(event RuntimeEvent) {
	if r != nil && r.Config.OnEvent != nil {
		r.Config.OnEvent(event)
	}
}
