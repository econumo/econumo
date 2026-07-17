package transaction

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// PageCursor is a position in the (spent_at DESC, id ASC) transaction order:
// the row it points at was the last one already returned.
type PageCursor struct {
	SpentAt time.Time
	ID      vo.Id
}

// EncodeCursor serializes a cursor as base64url("spent_at|id"). Exported so the
// api-parity catalogue can build deterministic cursors from fixture constants.
func EncodeCursor(c PageCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.SpentAt.Format(datetime.Layout) + "|" + c.ID.String()))
}

func invalidCursor() error {
	return errs.NewValidation("Form validation error",
		errs.FieldError{Key: "cursor", Message: "This value is not a valid cursor."})
}

func decodeCursor(raw string) (PageCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return PageCursor{}, invalidCursor()
	}
	at, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return PageCursor{}, invalidCursor()
	}
	spentAt, err := time.Parse(datetime.Layout, at)
	if err != nil {
		return PageCursor{}, invalidCursor()
	}
	parsed, err := vo.ParseId(id)
	if err != nil {
		return PageCursor{}, invalidCursor()
	}
	return PageCursor{SpentAt: spentAt, ID: parsed}, nil
}
