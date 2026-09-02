package model_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestImportTransactionLink_SeenAndTombstone(t *testing.T) {
	tx := vo.NewId()
	cases := []struct {
		name      string
		link      model.ImportTransactionLink
		seen      bool
		tombstone bool
	}{
		{"linked live", model.ImportTransactionLink{Status: model.ImportLinkStatusLinked, TransactionID: &tx}, true, false},
		{"linked deleted", model.ImportTransactionLink{Status: model.ImportLinkStatusLinked}, true, true},
		{"skipped", model.ImportTransactionLink{Status: model.ImportLinkStatusSkipped}, true, false},
		{"queued", model.ImportTransactionLink{Status: model.ImportLinkStatusQueued}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.link.IsSeen() != tc.seen {
				t.Errorf("IsSeen = %v, want %v", tc.link.IsSeen(), tc.seen)
			}
			if tc.link.IsTombstone() != tc.tombstone {
				t.Errorf("IsTombstone = %v, want %v", tc.link.IsTombstone(), tc.tombstone)
			}
		})
	}
}
