package reqctx

import (
	"context"
	"testing"
	"time"
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

func TestLanguageDefaultsToEnglish(t *testing.T) {
	if got := Language(context.Background()); got != "en" {
		t.Fatalf("Language() = %q, want en", got)
	}
}

func TestLanguageRoundTrip(t *testing.T) {
	ctx := WithLanguage(context.Background(), "ru")
	if got := Language(ctx); got != "ru" {
		t.Fatalf("Language() = %q, want ru", got)
	}
}

func TestExplicitLocation(t *testing.T) {
	ctx := context.Background()
	if IsLocationExplicit(ctx) {
		t.Fatal("empty ctx must not be explicit")
	}
	// Plain WithLocation (the Timezone middleware's UTC default) is NOT explicit.
	ctx = WithLocation(ctx, time.UTC)
	if IsLocationExplicit(ctx) {
		t.Fatal("WithLocation must not mark explicit")
	}
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	ctx = WithExplicitLocation(ctx, loc)
	if !IsLocationExplicit(ctx) {
		t.Fatal("WithExplicitLocation must mark explicit")
	}
	if got := Location(ctx); got.String() != "Europe/Amsterdam" {
		t.Fatalf("Location = %s, want Europe/Amsterdam", got)
	}
}
