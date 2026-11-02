
package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"

	ort "github.com/microsoft/onnxruntime/go/onnxruntime"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (retErr error) {
	modelPath := "embedding/models/bge-small-en-v1.5/model.onnx"
	runtimePath := "third_party/onnxruntime/onnxruntime-linux-x64-1.31.1/lib/libonnxruntime.so"

	// Resolve paths relative to the api directory.
	modelPath, _ = filepath.Abs(modelPath)
	runtimePath, _ = filepath.Abs(runtimePath)

	if _, err := os.Stat(modelPath); err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if _, err := os.Stat(runtimePath); err != nil {
		return fmt.Errorf("ONNX Runtime library not found: %w", err)
	}

	// Initialize ONNX Runtime.
	ort.SetSharedLibraryPath(runtimePath)

	if err := ort.Init(); err != nil {
		return fmt.Errorf("ONNX Runtime initialization failed: %w", err)
	}

	// Shutdown only after all sessions and tensors have been closed.
	defer func() {
		if err := ort.Shutdown(); err != nil && retErr == nil {
			retErr = fmt.Errorf("ONNX Runtime shutdown failed: %w", err)
		}
	}()

	version, err := ort.GetVersion()
	if err != nil {
		return err
	}

	fmt.Println("ONNX Runtime version:", version)
	fmt.Println("Negotiated C API version:", ort.APIVersion())

	// Load the actual BGE model.
	fmt.Println("\nLoading BGE model...")

	session, err := ort.NewSession(modelPath, nil)
	if err != nil {
		return fmt.Errorf("failed to load BGE model: %w", err)
	}
	defer session.Close()

	fmt.Println("BGE model loaded successfully.")

	// Inspect model inputs.
	fmt.Println("\nModel inputs:")
	inputsInfo := session.Inputs()

	for _, input := range inputsInfo {
		fmt.Printf("  %s | type=%s | shape=%v\n",
			input.Name, input.DataType, input.Shape)
	}

	// Inspect model outputs.
	fmt.Println("\nModel outputs:")
	for _, output := range session.Outputs() {
		fmt.Printf("  %s | type=%s | shape=%v\n",
			output.Name, output.DataType, output.Shape)
	}

	// Basic valid BERT token IDs:
	// [CLS] hello world [SEP]
	//
	// This is a runtime smoke test, not the production tokenizer.
	inputIDs := []int64{101, 7592, 2088, 102}
	attentionMask := []int64{1, 1, 1, 1}
	tokenTypeIDs := []int64{0, 0, 0, 0}

	shape := []int64{1, 4}

	idsTensor, err := ort.CreateTensor(shape, inputIDs)
	if err != nil {
		return fmt.Errorf("input_ids tensor: %w", err)
	}
	defer idsTensor.Close()

	maskTensor, err := ort.CreateTensor(shape, attentionMask)
	if err != nil {
		return fmt.Errorf("attention_mask tensor: %w", err)
	}
	defer maskTensor.Close()

	typeTensor, err := ort.CreateTensor(shape, tokenTypeIDs)
	if err != nil {
		return fmt.Errorf("token_type_ids tensor: %w", err)
	}
	defer typeTensor.Close()

	// Verify that the expected model inputs exist.
	inputMap := map[string]bool{}

	for _, input := range inputsInfo {
		inputMap[input.Name] = true
	}

	for _, name := range []string{
		"input_ids",
		"attention_mask",
		"token_type_ids",
	} {
		if !inputMap[name] {
			return fmt.Errorf("expected model input %q not found", name)
		}
	}

	// Execute actual ONNX inference.
	fmt.Println("\nRunning inference...")

	outputs, err := session.Run(
		context.Background(),
		map[string]*ort.Tensor{
			"input_ids":      idsTensor,
			"attention_mask": maskTensor,
			"token_type_ids": typeTensor,
		},
		[]string{"last_hidden_state"},
	)
	if err != nil {
		return fmt.Errorf("inference failed: %w", err)
	}

	defer func() {
		for _, tensor := range outputs {
			tensor.Close()
		}
	}()

	result, ok := outputs["last_hidden_state"]
	if !ok {
		return fmt.Errorf("last_hidden_state missing from model outputs")
	}

	// Validate output dimensions.
	outputShape := result.Shape()

	fmt.Println("\nOutput shape:", outputShape)

	if len(outputShape) != 3 || outputShape[2] != 384 {
		return fmt.Errorf(
			"unexpected output shape: expected [batch, sequence, 384], got %v",
			outputShape,
		)
	}

	// Read output data.
	data, err := ort.TensorData[float32](result)
	if err != nil {
		return fmt.Errorf("failed to read output tensor: %w", err)
	}

	if len(data) != int(outputShape[0]*outputShape[1]*outputShape[2]) {
		return fmt.Errorf("output element count does not match shape")
	}

	// Validate numerical output.
	for i, value := range data {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("invalid output value at index %d: %v", i, value)
		}
	}

	fmt.Println("Output elements:", len(data))
	fmt.Println("All output values are finite.")
	fmt.Printf("First 10 values: %v\n", data[:10])

	fmt.Println("\nSUCCESS: BGE ONNX inference executed successfully.")

	return nil
}
