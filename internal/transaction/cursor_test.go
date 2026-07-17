package transaction

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestCursorRoundTrip(t *testing.T) {
	in := PageCursor{
		SpentAt: time.Date(2026, 3, 5, 10, 30, 0, 0, time.UTC),
		ID:      vo.MustParseId("d0000000-0000-0000-0000-000000000001"),
	}
	raw := EncodeCursor(in)
	out, err := decodeCursor(raw)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if !out.SpentAt.Equal(in.SpentAt) || !out.ID.Equal(in.ID) {
		t.Fatalf("round trip = %+v, want %+v", out, in)
	}
}

func TestDecodeCursor_Invalid(t *testing.T) {
	for _, raw := range []string{
		"%%%not-base64",
		"aGVsbG8",              // decodes but has no separator
		"MjAyNi0wMy0wNXxub3Bl", // "2026-03-05|nope": bad datetime AND bad id
	} {
		_, err := decodeCursor(raw)
		if err == nil {
			t.Fatalf("decodeCursor(%q): want error", raw)
		}
		ve, ok := errs.AsValidation(err)
		if !ok {
			t.Fatalf("decodeCursor(%q): err type %T, want *errs.ValidationError", raw, err)
		}
		if ve.Msg != "Form validation error" {
			t.Errorf("decodeCursor(%q): Msg = %q, want %q", raw, ve.Msg, "Form validation error")
		}
		if len(ve.Fields) != 1 || ve.Fields[0].Key != "cursor" || ve.Fields[0].Message != "This value is not a valid cursor." {
			t.Errorf("decodeCursor(%q): Fields = %+v, want one {Key: %q, Message: %q}",
				raw, ve.Fields, "cursor", "This value is not a valid cursor.")
		}
	}
}
