package model_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestCreateLabelRequestValidate(t *testing.T) {
	if err := (model.CreateLabelRequest{Id: "", Name: "Kid"}).Validate(); err == nil {
		t.Fatal("blank id must fail validation")
	}
	if err := (model.CreateLabelRequest{Id: "op", Name: "  "}).Validate(); err == nil {
		t.Fatal("blank name must fail validation")
	}
	if err := (model.CreateLabelRequest{Id: "op", Name: "Kid"}).Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
}

func TestSortLabelListRequestRejectsEmpty(t *testing.T) {
	if err := (model.SortLabelListRequest{}).Validate(); err == nil {
		t.Fatal("empty ids list must fail validation")
	}
}

func TestMoveLabelRequestValidate(t *testing.T) {
	id := vo.NewId().String()
	if err := (model.MoveLabelRequest{Id: "not-a-uuid"}).Validate(); err == nil {
		t.Fatal("malformed id must fail validation")
	}
	bad := "not-a-uuid"
	if err := (model.MoveLabelRequest{Id: id, AfterId: &bad}).Validate(); err == nil {
		t.Fatal("malformed afterId must fail validation")
	}
	// A null afterId is the "move to front" anchor, not a missing field.
	if err := (model.MoveLabelRequest{Id: id}).Validate(); err != nil {
		t.Fatalf("null afterId rejected: %v", err)
	}
}
