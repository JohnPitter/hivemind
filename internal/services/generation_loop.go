package services

import (
	"context"
	"fmt"
	"time"

	"github.com/joaopedro/hivemind/gen/workerpb"
	"github.com/joaopedro/hivemind/internal/logger"
	"github.com/joaopedro/hivemind/internal/models"
)

// GenerationConfig holds parameters for distributed token generation.
type GenerationConfig struct {
	ModelID     string
	Prompt      string
	MaxTokens   int
	Temperature float32
	TopP        float32
	TopK        int32
	EosTokenIDs []int32
	RequestID   string
}

// GenerationUsageStats tracks token counts for a generation request.
type GenerationUsageStats struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// GenerateDistributed runs the distributed autoregressive generation loop
// synchronously, returning the complete generated text.
func (d *DistributedInferenceService) GenerateDistributed(
	ctx context.Context,
	cfg GenerationConfig,
) (string, GenerationUsageStats, error) {
	logger.Info("starting distributed generation",
		"request_id", cfg.RequestID,
		"model_id", cfg.ModelID,
		"max_tokens", cfg.MaxTokens,
	)

	d.generationRequests.Add(1)
	genStart := time.Now()

	// Step 1: Embed the prompt
	embedStart := time.Now()
	embedResp, err := d.embedTokens(ctx, &workerpb.EmbedRequest{
		RequestId: cfg.RequestID,
		Text:      cfg.Prompt,
		ModelId:   cfg.ModelID,
	})
	if err != nil {
		return "", GenerationUsageStats{}, fmt.Errorf("embed prompt failed: %w", err)
	}
	embedDuration := time.Since(embedStart).Milliseconds()
	d.embedTotalMs.Add(embedDuration)
	d.embedCount.Add(1)

	promptTokens := len(embedResp.TokenIds)
	hiddenStates := embedResp.HiddenStates

	logger.Info("prompt embedded",
		"request_id", cfg.RequestID,
		"prompt_tokens", promptTokens,
		"embed_ms", embedDuration,
	)

	// Step 2: Forward pass through all peers (initial pass — no cache)
	var cacheSeqLen int32
	hiddenStates, _, cacheSeqLen, err = d.ExecuteDistributedForwardPass(ctx, hiddenStates, cfg.RequestID, false, 0)
	if err != nil {
		return "", GenerationUsageStats{}, fmt.Errorf("initial forward pass failed: %w", err)
	}

	// Step 3: Autoregressive loop
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 256
	}

	var generatedText string
	completionTokens := 0

	for i := 0; i < maxTokens; i++ {
		tokenStart := time.Now()

		// Sample next token from the output hidden states
		sampleStart := time.Now()
		sampleResp, err := d.sampleTokens(ctx, &workerpb.SampleRequest{
			RequestId:   cfg.RequestID,
			Logits:      hiddenStates,
			Temperature: cfg.Temperature,
			TopP:        cfg.TopP,
			TopK:        cfg.TopK,
			EosTokenIds: cfg.EosTokenIDs,
		})
		if err != nil {
			return "", GenerationUsageStats{}, fmt.Errorf("sample token %d failed: %w", i, err)
		}
		sampleDuration := time.Since(sampleStart).Milliseconds()
		d.sampleTotalMs.Add(sampleDuration)
		d.sampleCount.Add(1)

		completionTokens++
		generatedText += sampleResp.TokenText

		// Check for EOS
		if sampleResp.IsEos {
			logger.Info("EOS token reached",
				"request_id", cfg.RequestID,
				"completion_tokens", completionTokens,
			)
			break
		}

		// Embed the new token
		embedStart = time.Now()
		embedResp, err = d.embedTokens(ctx, &workerpb.EmbedRequest{
			RequestId: cfg.RequestID,
			TokenIds:  []int32{sampleResp.TokenId},
			ModelId:   cfg.ModelID,
		})
		if err != nil {
			return "", GenerationUsageStats{}, fmt.Errorf("embed token %d failed: %w", i, err)
		}
		d.embedTotalMs.Add(time.Since(embedStart).Milliseconds())
		d.embedCount.Add(1)

		hiddenStates = embedResp.HiddenStates

		// Forward pass through all peers (single token — use KV cache)
		hiddenStates, _, cacheSeqLen, err = d.ExecuteDistributedForwardPass(ctx, hiddenStates, cfg.RequestID, true, cacheSeqLen)
		if err != nil {
			return "", GenerationUsageStats{}, fmt.Errorf("forward pass at token %d failed: %w", i, err)
		}

		tokenDuration := time.Since(tokenStart).Milliseconds()
		d.recordTokenLatency(tokenDuration)
		d.tokensGenerated.Add(1)
	}

	totalDuration := time.Since(genStart).Milliseconds()
	usage := GenerationUsageStats{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}

	logger.Info("distributed generation completed",
		"request_id", cfg.RequestID,
		"prompt_tokens", promptTokens,
		"completion_tokens", completionTokens,
		"total_ms", totalDuration,
	)

	return generatedText, usage, nil
}

// GenerateDistributedStream runs the distributed autoregressive generation loop,
// yielding tokens via a channel as they are generated.
func (d *DistributedInferenceService) GenerateDistributedStream(
	ctx context.Context,
	cfg GenerationConfig,
	tokenCh chan<- string,
) (GenerationUsageStats, error) {
	logger.Info("starting distributed generation (streaming)",
		"request_id", cfg.RequestID,
		"model_id", cfg.ModelID,
		"max_tokens", cfg.MaxTokens,
	)

	d.generationRequests.Add(1)
	genStart := time.Now()

	// Step 1: Embed the prompt
	embedStart := time.Now()
	embedResp, err := d.embedTokens(ctx, &workerpb.EmbedRequest{
		RequestId: cfg.RequestID,
		Text:      cfg.Prompt,
		ModelId:   cfg.ModelID,
	})
	if err != nil {
		return GenerationUsageStats{}, fmt.Errorf("embed prompt failed: %w", err)
	}
	d.embedTotalMs.Add(time.Since(embedStart).Milliseconds())
	d.embedCount.Add(1)

	promptTokens := len(embedResp.TokenIds)
	hiddenStates := embedResp.HiddenStates

	// Step 2: Initial forward pass (no cache)
	var cacheSeqLen int32
	hiddenStates, _, cacheSeqLen, err = d.ExecuteDistributedForwardPass(ctx, hiddenStates, cfg.RequestID, false, 0)
	if err != nil {
		return GenerationUsageStats{}, fmt.Errorf("initial forward pass failed: %w", err)
	}

	// Step 3: Autoregressive loop with streaming
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 256
	}

	completionTokens := 0

	for i := 0; i < maxTokens; i++ {
		tokenStart := time.Now()

		// Check context cancellation
		select {
		case <-ctx.Done():
			return GenerationUsageStats{
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      promptTokens + completionTokens,
			}, ctx.Err()
		default:
		}

		// Sample next token
		sampleStart := time.Now()
		sampleResp, err := d.sampleTokens(ctx, &workerpb.SampleRequest{
			RequestId:   cfg.RequestID,
			Logits:      hiddenStates,
			Temperature: cfg.Temperature,
			TopP:        cfg.TopP,
			TopK:        cfg.TopK,
			EosTokenIds: cfg.EosTokenIDs,
		})
		if err != nil {
			return GenerationUsageStats{}, fmt.Errorf("sample token %d failed: %w", i, err)
		}
		d.sampleTotalMs.Add(time.Since(sampleStart).Milliseconds())
		d.sampleCount.Add(1)

		completionTokens++

		// Yield token to channel
		if sampleResp.TokenText != "" {
			select {
			case tokenCh <- sampleResp.TokenText:
			case <-ctx.Done():
				return GenerationUsageStats{
					PromptTokens:     promptTokens,
					CompletionTokens: completionTokens,
					TotalTokens:      promptTokens + completionTokens,
				}, ctx.Err()
			}
		}

		if sampleResp.IsEos {
			break
		}

		// Embed the new token
		embedStart = time.Now()
		embedResp, err = d.embedTokens(ctx, &workerpb.EmbedRequest{
			RequestId: cfg.RequestID,
			TokenIds:  []int32{sampleResp.TokenId},
			ModelId:   cfg.ModelID,
		})
		if err != nil {
			return GenerationUsageStats{}, fmt.Errorf("embed token %d failed: %w", i, err)
		}
		d.embedTotalMs.Add(time.Since(embedStart).Milliseconds())
		d.embedCount.Add(1)

		hiddenStates = embedResp.HiddenStates

		// Forward pass through all peers (use KV cache)
		hiddenStates, _, cacheSeqLen, err = d.ExecuteDistributedForwardPass(ctx, hiddenStates, cfg.RequestID, true, cacheSeqLen)
		if err != nil {
			return GenerationUsageStats{}, fmt.Errorf("forward pass at token %d failed: %w", i, err)
		}

		tokenDuration := time.Since(tokenStart).Milliseconds()
		d.recordTokenLatency(tokenDuration)
		d.tokensGenerated.Add(1)
	}

	totalDuration := time.Since(genStart).Milliseconds()
	usage := GenerationUsageStats{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}

	logger.Info("distributed generation (streaming) completed",
		"request_id", cfg.RequestID,
		"prompt_tokens", promptTokens,
		"completion_tokens", completionTokens,
		"total_ms", totalDuration,
	)

	return usage, nil
}

// embedTokens calls the local worker's EmbedTokens RPC.
func (d *DistributedInferenceService) embedTokens(ctx context.Context, req *workerpb.EmbedRequest) (*workerpb.EmbedResponse, error) {
	client := d.localWorker()
	if client == nil {
		return nil, models.ErrWorkerUnavail
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return client.EmbedTokens(ctx, req)
}

// sampleTokens calls the local worker's SampleTokens RPC.
func (d *DistributedInferenceService) sampleTokens(ctx context.Context, req *workerpb.SampleRequest) (*workerpb.SampleResponse, error) {
	client := d.localWorker()
	if client == nil {
		return nil, models.ErrWorkerUnavail
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return client.SampleTokens(ctx, req)
}
