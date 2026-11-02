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
}

func Load(cfg Config) (*Embedder, error) {
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
	outName := "last_hidden_state"
	if len(outputs) != 1 || outputs[0].Dimensions[len(outputs[0].Dimensions)-1] != int64(cfg.Dimensions) {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("embedding: expected token output dimension %d", cfg.Dimensions)
	}
	tok, err := pretrained.FromFile(cfg.TokenizerPath)
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("embedding: load tokenizer %s: %w", cfg.TokenizerPath, err)
	}
	session, err := ort.NewDynamicAdvancedSession(cfg.ModelPath, inputNames, []string{outName}, nil)
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("embedding: create model session: %w", err)
	}
	e := &Embedder{tok: tok, session: session, inputNames: inputNames, outName: outName, dimensions: cfg.Dimensions, batchSize: cfg.BatchSize}
	if _, err := e.Embed(context.Background(), []string{"model startup warmup"}); err != nil {
		_ = e.Close()
		return nil, fmt.Errorf("embedding: warmup: %w", err)
	}
	return e, nil
}

type Config struct {
	ModelPath, TokenizerPath, RuntimePath string
	Dimensions, BatchSize                 int
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
			if enc.Len() > 512 {
				enc, err = enc.Truncate(512, 0)
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
			for j, id := range enc.Ids {
				if id < 0 || id > math.MaxInt32 {
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
			inputVals[i] = values[n]
		}
		outs := []ort.Value{nil}
		err = e.session.Run(inputVals, outs)
		ti.Destroy()
		tm.Destroy()
		tt.Destroy()
		if err != nil {
			return nil, err
		}
		tensor, ok := outs[0].(*ort.Tensor[float32])
		if !ok {
			outs[0].Destroy()
			return nil, fmt.Errorf("embedding: unexpected output type")
		}
		data := tensor.GetData()
		dims := tensor.GetShape()
		if len(dims) != 3 || dims[2] != int64(e.dimensions) {
			tensor.Destroy()
			return nil, fmt.Errorf("embedding: unexpected output shape %v", dims)
		}
		for row := 0; row < end-base; row++ {
			pooled := make([]float32, e.dimensions)
			var count float32
			for col := 0; col < maxLen; col++ {
				if masks[row*maxLen+col] == 0 {
					continue
				}
				count++
				for d := range pooled {
					pooled[d] += data[(row*maxLen+col)*e.dimensions+d]
				}
			}
			var norm float64
			for d := range pooled {
				pooled[d] /= count
				norm += float64(pooled[d]) * float64(pooled[d])
			}
			norm = math.Sqrt(norm)
			if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
				tensor.Destroy()
				return nil, fmt.Errorf("embedding: invalid vector norm")
			}
			for d := range pooled {
				pooled[d] /= float32(norm)
			}
			out[base+row] = pooled
		}
		tensor.Destroy()
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
