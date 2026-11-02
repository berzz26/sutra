
package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/sugarme/tokenizer/pretrained"
)

func main() {
	tokenizerPath := filepath.Join(
		"embedding",
		"models",
		"bge-small-en-v1.5",
		"tokenizer.json",
	)

	tokenizer, err := pretrained.FromFile(tokenizerPath)
	if err != nil {
		log.Fatalf("failed to load tokenizer: %v", err)
	}

	fmt.Println("BGE tokenizer loaded successfully.")

	text := "How does semantic search work in a video intelligence system?"

	encoding, err := tokenizer.EncodeSingle(text, true)
	if err != nil {
		log.Fatalf("tokenization failed: %v", err)
	}

	ids := encoding.GetIds()
	attentionMask := encoding.GetAttentionMask()
	tokenTypeIDs := encoding.GetTypeIds()

	fmt.Println("\nInput:", text)
	fmt.Println("Tokens:", encoding.GetTokens())
	fmt.Println("Input IDs:", ids)
	fmt.Println("Attention Mask:", attentionMask)
	fmt.Println("Token Type IDs:", tokenTypeIDs)

	if len(ids) == 0 {
		log.Fatal("tokenizer returned zero tokens")
	}

	if len(ids) != len(attentionMask) ||
		len(ids) != len(tokenTypeIDs) {
		log.Fatal("tokenizer output lengths do not match")
	}

	fmt.Println("\nToken count:", len(ids))
	fmt.Println("SUCCESS: BGE tokenization executed successfully.")
}

