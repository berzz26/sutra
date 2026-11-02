package embedding

import (
	"math"
	"testing"
)

func TestMeanPoolNormalizeUsesAttentionMask(t *testing.T) {
	got, err := meanPoolNormalize([]float32{1, 0, 3, 0, 100, 100}, []int64{1, 0}, 1, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	wantNorm := float64(math.Sqrt(10))
	if math.Abs(float64(got[0][0])-1/wantNorm) > 1e-6 || math.Abs(float64(got[0][1])-0) > 1e-6 || math.Abs(float64(got[0][2])-3/wantNorm) > 1e-6 {
		t.Fatalf("unexpected normalized pooled vector: %v", got[0])
	}
}

func TestMeanPoolNormalizeRejectsInvalidValues(t *testing.T) {
	if _, err := meanPoolNormalize([]float32{float32(math.NaN())}, []int64{1}, 1, 1, 1); err == nil {
		t.Fatal("expected non-finite output error")
	}
	if _, err := meanPoolNormalize([]float32{0}, []int64{0}, 1, 1, 1); err == nil {
		t.Fatal("expected all-padding error")
	}
}
