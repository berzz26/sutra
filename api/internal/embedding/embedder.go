package embedding

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
	ort "github.com/yalue/onnxruntime_go"
)

type Embedder struct {
	mu                    sync.Mutex
	tok                   *tokenizer.Tokenizer
	session               *ort.DynamicAdvancedSession
	inputNames            []string
	outName               string
	dimensions, batchSize int
	maxTokens             int
}

func Load(cfg Config) (*Embedder, error) {
	if cfg.Dimensions <= 0 || cfg.BatchSize <= 0 {
		return nil, fmt.Errorf("embedding: dimensions and batch size must be positive")
	}
	if cfg.MaxTokens <= 0 || cfg.MaxTokens > 512 {
		return nil, fmt.Errorf("embedding: max tokens must be between 1 and 512")
	}
	for _, path := range []string{cfg.ModelPath, cfg.TokenizerPath} {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("embedding: artifact %s: %w", path, err)
		}
	}
	if cfg.RuntimePath != "" {
		ort.SetSharedLibraryPath(cfg.RuntimePath)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("embedding: initialize ONNX runtime: %w", err)
	}
	inputs, outputs, err := ort.GetInputOutputInfo(cfg.ModelPath)
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("embedding: inspect model: %w", err)
	}
	inputNames := make([]string, len(inputs))
	for i, in := range inputs {
		inputNames[i] = in.Name
	}
	if len(outputs) != 1 || len(outputs[0].Dimensions) != 3 || outputs[0].Dimensions[2] != int64(cfg.Dimensions) {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("embedding: expected token output dimension %d", cfg.Dimensions)
	}
	tok, err := pretrained.FromFile(cfg.TokenizerPath)
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("embedding: load tokenizer %s: %w", cfg.TokenizerPath, err)
	}
	options, err := ort.NewSessionOptions()
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("embedding: create session options: %w", err)
	}
	if cfg.Threads > 0 {
		if err := options.SetIntraOpNumThreads(cfg.Threads); err != nil {
			_ = options.Destroy()
			_ = ort.DestroyEnvironment()
			return nil, fmt.Errorf("embedding: set ONNX threads: %w", err)
		}
	}
	session, err := ort.NewDynamicAdvancedSession(cfg.ModelPath, inputNames, []string{outputs[0].Name}, options)
	_ = options.Destroy()
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("embedding: create model session: %w", err)
	}
	for _, required := range []string{"input_ids", "attention_mask", "token_type_ids"} {
		found := false
		for _, name := range inputNames {
			if name == required {
				found = true
				break
			}
		}
		if !found {
			_ = session.Destroy()
			_ = ort.DestroyEnvironment()
			return nil, fmt.Errorf("embedding: model input %q is missing", required)
		}
	}
	e := &Embedder{tok: tok, session: session, inputNames: inputNames, outName: outputs[0].Name, dimensions: cfg.Dimensions, batchSize: cfg.BatchSize, maxTokens: cfg.MaxTokens}
	if _, err := e.Embed(context.Background(), []string{"model startup warmup"}); err != nil {
		_ = e.Close()
		return nil, fmt.Errorf("embedding: warmup: %w", err)
	}
	return e, nil
}

type Config struct {
	ModelPath, TokenizerPath, RuntimePath string
	Dimensions, BatchSize                 int
	MaxTokens                             int
	Threads                               int
}

func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([][]float32, len(texts))
	for base := 0; base < len(texts); base += e.batchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := base + e.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		encs := make([]*tokenizer.Encoding, end-base)
		maxLen := 0
		for i, text := range texts[base:end] {
			enc, err := e.tok.EncodeSingle(text, true)
			if err != nil {
				return nil, err
			}
			if enc.Len() > e.maxTokens {
				enc, err = enc.Truncate(e.maxTokens, 0)
				if err != nil {
					return nil, err
				}
			}
			encs[i] = enc
			if enc.Len() > maxLen {
				maxLen = enc.Len()
			}
		}
		ids := make([]int64, (end-base)*maxLen)
		masks := make([]int64, len(ids))
		types := make([]int64, len(ids))
		for i, enc := range encs {
			if len(enc.AttentionMask) != len(enc.Ids) {
				return nil, fmt.Errorf("embedding: tokenizer returned mismatched ids and attention mask lengths")
			}
			for j, id := range enc.Ids {
				if id < 0 || uint64(id) > math.MaxInt32 {
					return nil, fmt.Errorf("embedding: invalid token id %d", id)
				}
				ids[i*maxLen+j] = int64(id)
				masks[i*maxLen+j] = int64(enc.AttentionMask[j])
				if j < len(enc.TypeIds) {
					types[i*maxLen+j] = int64(enc.TypeIds[j])
				}
			}
		}
		shape := ort.NewShape(int64(end-base), int64(maxLen))
		ti, err := ort.NewTensor(shape, ids)
		if err != nil {
			return nil, err
		}
		tm, err := ort.NewTensor(shape, masks)
		if err != nil {
			ti.Destroy()
			return nil, err
		}
		tt, err := ort.NewTensor(shape, types)
		if err != nil {
			ti.Destroy()
			tm.Destroy()
			return nil, err
		}
		values := map[string]ort.Value{"input_ids": ti, "attention_mask": tm, "token_type_ids": tt}
		inputVals := make([]ort.Value, len(e.inputNames))
		for i, n := range e.inputNames {
			value, ok := values[n]
			if !ok {
				ti.Destroy()
				tm.Destroy()
				tt.Destroy()
				return nil, fmt.Errorf("embedding: unsupported model input %q", n)
			}
			inputVals[i] = value
		}
		outs := []ort.Value{nil}
		err = e.session.Run(inputVals, outs)
		ti.Destroy()
		tm.Destroy()
		tt.Destroy()
		if err != nil {
			return nil, fmt.Errorf("embedding: inference batch %d-%d: %w", base, end, err)
		}
		tensor, ok := outs[0].(*ort.Tensor[float32])
		if !ok {
			if outs[0] != nil {
				outs[0].Destroy()
			}
			return nil, fmt.Errorf("embedding: unexpected output type")
		}
		data := tensor.GetData()
		dims := tensor.GetShape()
		if len(dims) != 3 || dims[2] != int64(e.dimensions) {
			tensor.Destroy()
			return nil, fmt.Errorf("embedding: unexpected output shape %v", dims)
		}
		if len(data) != (end-base)*maxLen*e.dimensions {
			tensor.Destroy()
			return nil, fmt.Errorf("embedding: output element count %d does not match shape %v", len(data), dims)
		}
		pooled, err := meanPoolNormalize(data, masks, end-base, maxLen, e.dimensions)
		if err != nil {
			tensor.Destroy()
			return nil, err
		}
		copy(out[base:end], pooled)
		tensor.Destroy()
	}
	return out, nil
}

func meanPoolNormalize(data []float32, masks []int64, rows, tokens, dimensions int) ([][]float32, error) {
	if rows <= 0 || tokens <= 0 || dimensions <= 0 || len(data) != rows*tokens*dimensions || len(masks) != rows*tokens {
		return nil, fmt.Errorf("embedding: invalid pooling input shape")
	}
	out := make([][]float32, rows)
	for row := 0; row < rows; row++ {
		pooled := make([]float32, dimensions)
		var count float32
		for token := 0; token < tokens; token++ {
			if masks[row*tokens+token] == 0 {
				continue
			}
			count++
			for dimension := 0; dimension < dimensions; dimension++ {
				value := data[(row*tokens+token)*dimensions+dimension]
				if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
					return nil, fmt.Errorf("embedding: non-finite model output at row %d token %d dimension %d", row, token, dimension)
				}
				pooled[dimension] += value
			}
		}
		if count == 0 {
			return nil, fmt.Errorf("embedding: tokenizer produced an all-padding row")
		}
		var norm float64
		for dimension := range pooled {
			pooled[dimension] /= count
			norm += float64(pooled[dimension]) * float64(pooled[dimension])
		}
		norm = math.Sqrt(norm)
		if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
			return nil, fmt.Errorf("embedding: invalid vector norm")
		}
		for dimension := range pooled {
			pooled[dimension] /= float32(norm)
		}
		out[row] = pooled
	}
	return out, nil
}

func (e *Embedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session != nil {
		_ = e.session.Destroy()
		e.session = nil
	}
	return ort.DestroyEnvironment()
}
