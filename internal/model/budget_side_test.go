package model

import "testing"

func TestElementTypeIncomeValues(t *testing.T) {
	if ElementIncomeCategory != 3 || ElementIncomeEnvelope != 4 {
		t.Fatalf("income element type values are frozen: category=3 envelope=4, got %d/%d",
			ElementIncomeCategory, ElementIncomeEnvelope)
	}
	if ElementIncomeCategory.Alias() != "income_category" || ElementIncomeEnvelope.Alias() != "income_envelope" {
		t.Fatalf("aliases: got %q / %q", ElementIncomeCategory.Alias(), ElementIncomeEnvelope.Alias())
	}
}

func TestElementTypeFromAlias_RejectsIncomeAliases(t *testing.T) {
	// The income aliases are internal-only; the wire parser keeps rejecting them
	// so existing drill-down validation output is unchanged.
	for _, alias := range []string{"income_category", "income_envelope"} {
		if _, err := ElementTypeFromAlias(alias); err == nil {
			t.Fatalf("alias %q must stay invalid on the wire", alias)
		}
	}
	for _, alias := range []string{"envelope", "category", "tag"} {
		if _, err := ElementTypeFromAlias(alias); err != nil {
			t.Fatalf("alias %q must stay valid: %v", alias, err)
		}
	}
}

func TestIsIncomeSide(t *testing.T) {
	want := map[ElementType]bool{
		ElementEnvelope: false, ElementCategory: false, ElementTag: false,
		ElementIncomeCategory: true, ElementIncomeEnvelope: true,
	}
	for typ, w := range want {
		if typ.IsIncomeSide() != w {
			t.Errorf("IsIncomeSide(%d)=%v want %v", typ, typ.IsIncomeSide(), w)
		}
	}
}

func TestEnvelopeTypeFromSide(t *testing.T) {
	for side, wantTyp := range map[string]ElementType{
		"": ElementEnvelope, "expense": ElementEnvelope, "income": ElementIncomeEnvelope,
	} {
		got, err := EnvelopeTypeFromSide(side)
		if err != nil || got != wantTyp {
			t.Errorf("EnvelopeTypeFromSide(%q) = %v, %v; want %v, nil", side, got, err, wantTyp)
		}
	}
	if _, err := EnvelopeTypeFromSide("both"); err == nil {
		t.Fatalf("invalid side must fail validation")
	}
}
