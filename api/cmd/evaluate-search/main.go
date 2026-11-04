// Command evaluate-search measures retrieval against the small labeled fixture
// in queries.json. The labels describe the indexed development dataset and
// should be replaced when a deployment creates its own evaluation set.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/berzz/searcher/backend/internal/config"
	"github.com/berzz/searcher/backend/internal/embedding"
	"github.com/berzz/searcher/backend/internal/persistence"
	"github.com/berzz/searcher/backend/internal/retrieval"
)

//go:embed queries.json
var fixtureFS embed.FS

type evalCase struct {
	Query            string   `json:"query"`
	RelevantChunkIDs []string `json:"relevant_chunk_ids"`
}

type metric struct {
	RecallAt5  float64 `json:"recall_at_5"`
	RecallAt10 float64 `json:"recall_at_10"`
	MRR        float64 `json:"mrr"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var cases []evalCase
	data, err := fixtureFS.ReadFile("queries.json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := persistence.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	emb, err := embedding.Load(embedding.Config{ModelPath: cfg.Embedding.ONNXPath, TokenizerPath: cfg.Embedding.TokenizerPath, RuntimePath: cfg.Embedding.RuntimePath, Dimensions: cfg.Embedding.Dimensions, BatchSize: cfg.Embedding.BatchSize, MaxTokens: cfg.Embedding.MaxTokens, Threads: cfg.Embedding.Threads})
	if err != nil {
		return err
	}
	defer emb.Close()
	service := retrieval.NewService(pool, emb, retrieval.Config{CandidatePool: cfg.Search.CandidatePool, RRFK: cfg.Search.RRFK, Timeout: cfg.Search.Timeout})
	all := map[string][]metric{"vector_only": {}, "lexical_only": {}, "hybrid": {}}
	for _, item := range cases {
		semantic, lexical, err := service.Candidates(context.Background(), item.Query, "")
		if err != nil {
			return fmt.Errorf("%q: %w", item.Query, err)
		}
		hybrid := retrieval.Fuse(semantic, lexical, cfg.Search.RRFK, 10, 0)
		all["vector_only"] = append(all["vector_only"], measure(candidateIDs(semantic), item.RelevantChunkIDs))
		all["lexical_only"] = append(all["lexical_only"], measure(candidateIDs(lexical), item.RelevantChunkIDs))
		hybridIDs := make([]string, len(hybrid))
		for i, result := range hybrid {
			hybridIDs[i] = result.ChunkID
		}
		all["hybrid"] = append(all["hybrid"], measure(hybridIDs, item.RelevantChunkIDs))
	}
	averages := make(map[string]metric, len(all))
	for name, values := range all {
		averages[name] = average(values)
	}
	encoded, _ := json.MarshalIndent(averages, "", "  ")
	fmt.Println(string(encoded))
	return nil
}

func candidateIDs(candidates []retrieval.Candidate) []string {
	ids := make([]string, len(candidates))
	for i, candidate := range candidates {
		ids[i] = candidate.ChunkID
	}
	return ids
}

func measure(ranked, relevant []string) metric {
	gold := make(map[string]struct{}, len(relevant))
	for _, id := range relevant {
		gold[id] = struct{}{}
	}
	hits := func(limit int) float64 {
		if len(ranked) < limit {
			limit = len(ranked)
		}
		count := 0
		seen := make(map[string]struct{})
		for _, id := range ranked[:limit] {
			if _, ok := gold[id]; ok {
				if _, duplicate := seen[id]; !duplicate {
					count++
				}
				seen[id] = struct{}{}
			}
		}
		return float64(count) / float64(len(gold))
	}
	result := metric{RecallAt5: hits(5), RecallAt10: hits(10)}
	for rank, id := range ranked {
		if _, ok := gold[id]; ok {
			result.MRR = 1 / float64(rank+1)
			break
		}
	}
	return result
}

func average(values []metric) metric {
	if len(values) == 0 {
		return metric{}
	}
	var result metric
	for _, value := range values {
		result.RecallAt5 += value.RecallAt5
		result.RecallAt10 += value.RecallAt10
		result.MRR += value.MRR
	}
	count := float64(len(values))
	result.RecallAt5 /= count
	result.RecallAt10 /= count
	result.MRR /= count
	return result
}
