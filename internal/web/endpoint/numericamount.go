package endpoint

import (
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/middleware"
)

var flexStringType = reflect.TypeOf(vo.FlexString{})

// warnNumericAmounts emits one WARN when a decoded request carried any money
// field as a JSON number — the deprecated lenient form the contract still
// accepts. Field names only, never values (amounts are user financial data).
func warnNumericAmounts(r *http.Request, req any) {
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	var fields []string
	for i := 0; i < t.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() == reflect.Pointer {
			if f.IsNil() {
				continue
			}
			f = f.Elem()
		}
		if f.Type() != flexStringType {
			continue
		}
		if !f.Interface().(vo.FlexString).FromNumber() {
			continue
		}
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" {
			name = t.Field(i).Name
		}
		fields = append(fields, name)
	}
	if len(fields) == 0 {
		return
	}
	slog.Warn("deprecated numeric amount",
		slog.String("route", r.Pattern),
		slog.String("request_id", middleware.RequestIDFromCtx(r.Context())),
		slog.String("fields", strings.Join(fields, ",")))
}
