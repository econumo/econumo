package reqctx

import (
	"context"
	"testing"
)

func TestLogAttrsAccumulate(t *testing.T) {
	ctx := WithLogAttrs(context.Background())
	AddLogAttr(ctx, "user_id", "u-1")
	AddLogAttr(ctx, "category_id", "c-2")

	attrs := LogAttrs(ctx)
	if len(attrs) != 2 {
		t.Fatalf("got %d attrs, want 2", len(attrs))
	}
	got := map[string]string{}
	for _, a := range attrs {
		got[a.Key] = a.Value.String()
	}
	if got["user_id"] != "u-1" || got["category_id"] != "c-2" {
		t.Errorf("attrs = %v", got)
	}
}

func TestAddLogAttrNoAccumulatorIsNoop(t *testing.T) {
	ctx := context.Background()
	// Must not panic and must report nothing collected.
	AddLogAttr(ctx, "user_id", "u-1")
	if attrs := LogAttrs(ctx); attrs != nil {
		t.Errorf("LogAttrs without accumulator = %v, want nil", attrs)
	}
}

func TestLogAttrsReturnsCopy(t *testing.T) {
	ctx := WithLogAttrs(context.Background())
	AddLogAttr(ctx, "a", "1")
	snapshot := LogAttrs(ctx)
	AddLogAttr(ctx, "b", "2") // mutating after the snapshot must not affect it
	if len(snapshot) != 1 {
		t.Errorf("snapshot len = %d, want 1 (LogAttrs must return a copy)", len(snapshot))
	}
}
