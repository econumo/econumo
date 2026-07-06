package vo

import (
	"encoding/json"
	"os"
	"testing"
)

// decimalVectors mirrors testdata/decimal_vectors.json: the frozen
// byte-compatibility oracle for Add/Sub/Mul/Div/Round at fixed scale 8.
type decimalVectors struct {
	Arith []struct {
		A, B, Add, Sub, Mul string
		Div                 *string
	} `json:"arith"`
	Round []struct {
		Value     string
		Precision int
		Result    string
	} `json:"round"`
}

func loadDecimalVectors(t *testing.T) decimalVectors {
	t.Helper()
	raw, err := os.ReadFile("testdata/decimal_vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var v decimalVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("parse vectors: %v", err)
	}
	if len(v.Arith) == 0 || len(v.Round) == 0 {
		t.Fatal("empty vectors")
	}
	return v
}

func TestDecimal_Arithmetic_GoldenVsPHP(t *testing.T) {
	v := loadDecimalVectors(t)
	for _, c := range v.Arith {
		a, b := NewDecimal(c.A), NewDecimal(c.B)
		if got := a.Add(b).String(); got != c.Add {
			t.Errorf("%s + %s = %s, want %s", c.A, c.B, got, c.Add)
		}
		if got := a.Sub(b).String(); got != c.Sub {
			t.Errorf("%s - %s = %s, want %s", c.A, c.B, got, c.Sub)
		}
		if got := a.Mul(b).String(); got != c.Mul {
			t.Errorf("%s * %s = %s, want %s", c.A, c.B, got, c.Mul)
		}
		if c.Div != nil {
			if got := a.Div(b).String(); got != *c.Div {
				t.Errorf("%s / %s = %s, want %s", c.A, c.B, got, *c.Div)
			}
		}
	}
}

func TestDecimal_Round_GoldenVsPHP(t *testing.T) {
	v := loadDecimalVectors(t)
	for _, c := range v.Round {
		if got := NewDecimal(c.Value).Round(c.Precision).String(); got != c.Result {
			t.Errorf("round(%s, %d) = %s, want %s", c.Value, c.Precision, got, c.Result)
		}
	}
}
