package apiparity

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/infra/handoff"
)

// billingAccessStub answers billing's get-access in place of the portal: it
// verifies the econumo-access bearer as billing would, then reports the owner
// as a renewing subscriber and everyone else as unpaid, echoing the
// connections Econumo asked about.
type billingAccessStub struct{ key string }

func (s billingAccessStub) RoundTrip(r *http.Request) (*http.Response, error) {
	reply := func(status int, body any) (*http.Response, error) {
		raw, _ := json.Marshal(body)
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(bytes.NewReader(raw)), Request: r}, nil
	}
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/cloud/api/v1/econumo/get-access") {
		return reply(http.StatusNotFound, map[string]any{"success": false})
	}
	uid, err := handoff.VerifyAccess(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), s.key, time.Now())
	if err != nil {
		return reply(http.StatusUnauthorized, map[string]any{"success": false})
	}
	var req struct {
		Connections []string `json:"connections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return reply(http.StatusBadRequest, map[string]any{"success": false})
	}
	user := map[string]any{"paid": false, "renewing": false, "renewsAt": "", "endsAt": "", "paymentFailed": false, "discountUntil": ""}
	if uid == OwnerID {
		user = map[string]any{"paid": true, "renewing": true, "renewsAt": "2026-12-24 10:00:00", "endsAt": "", "paymentFailed": false, "discountUntil": "2027-11-24 10:00:00"}
	}
	conns := []map[string]any{}
	for _, id := range req.Connections {
		conns = append(conns, map[string]any{"id": id, "paid": id == OwnerID, "renewing": id == OwnerID})
	}
	return reply(http.StatusOK, map[string]any{"success": true, "message": "", "data": map[string]any{"user": user, "connections": conns}})
}
