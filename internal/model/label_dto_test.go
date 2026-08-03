package model_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
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

func TestOrderLabelListRequestRejectsEmpty(t *testing.T) {
	if err := (model.OrderLabelListRequest{}).Validate(); err == nil {
		t.Fatal("empty changes list must fail validation")
	}
}
