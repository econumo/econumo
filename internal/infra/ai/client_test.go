package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestComplete_PostsChatCompletionAndReturnsContent(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"rules\":[]}"}}]}`)
	}))
	defer srv.Close()

	c := New(Config{Endpoint: srv.URL + "/v1", APIKey: "sk-test", Model: "gpt-5-mini"})
	out, err := c.Complete(context.Background(), "user-1", "SYS", "USER", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"rules":[]}` {
		t.Fatalf("content = %q", out)
	}
	if gotPath != "/v1/chat/completions" || gotAuth != "Bearer sk-test" {
		t.Fatalf("path=%q auth=%q", gotPath, gotAuth)
	}
	if gotBody["model"] != "gpt-5-mini" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	msgs := gotBody["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || msgs[1].(map[string]any)["content"] != "USER" {
		t.Fatalf("messages = %v", msgs)
	}
	if _, has := gotBody["response_format"]; has {
		t.Fatal("response_format must not be sent without a schema")
	}
	if _, has := gotBody["temperature"]; has {
		t.Fatal("temperature must be omitted (gpt-5/o-series reject any non-default value with a 400)")
	}
}

func TestComplete_KeylessSendsNoAuthorization(t *testing.T) {
	var gotAuth string
	var hasAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, hasAuth = r.Header.Get("Authorization"), len(r.Header.Values("Authorization")) > 0
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()
	if _, err := New(Config{Endpoint: srv.URL, Model: "llama3"}).Complete(context.Background(), "user-1", "s", "u", nil); err != nil {
		t.Fatal(err)
	}
	if hasAuth || gotAuth != "" {
		t.Fatalf("keyless client sent Authorization %q", gotAuth)
	}
}

func TestComplete_AnthropicPostsMessagesAndReturnsTextBlocks(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, key, system string
	}{
		{"hosted", "/v1", "sk-ant-test", "SYS"},
		{"gateway", "/custom/anthropic/v1/", "gateway-key", "SYS"},
		{"keyless without system", "/v1", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != strings.TrimRight(tc.prefix, "/")+"/messages" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("x-api-key") != tc.key || r.Header.Get("Authorization") != "" {
					t.Error("Anthropic must use x-api-key, without a bearer")
				}
				if tc.key == "" && len(r.Header.Values("x-api-key")) > 0 {
					t.Error("keyless gateway must not receive an empty x-api-key header")
				}
				if r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("headers = %v", r.Header)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body["model"] != "claude-sonnet-4-6" || body["max_tokens"] != float64(8192) {
					t.Errorf("model/budget = %v", body)
				}
				if tc.system != "" && body["system"] != tc.system {
					t.Errorf("system = %v", body["system"])
				}
				if tc.system == "" && body["system"] != nil {
					t.Error("empty system must be omitted")
				}
				msgs, _ := body["messages"].([]any)
				if len(msgs) != 1 {
					t.Errorf("messages = %v", msgs)
				} else if msg, _ := msgs[0].(map[string]any); msg["role"] != "user" || msg["content"] != "USER" {
					t.Errorf("user message = %v", msg)
				}
				for _, field := range []string{"temperature", "response_format", "output_config", "tools", "stream"} {
					if _, has := body[field]; has {
						t.Errorf("unexpected %s", field)
					}
				}
				_, _ = io.WriteString(w, `{"content":[{"type":"thinking","thinking":"private","text":"ignore"},{"type":"text","text":"{\"rules\":"},{"type":"text","text":"[]}"}],"stop_reason":"end_turn"}`)
			}))
			defer srv.Close()
			c := New(Config{Dialect: "anthropic", Endpoint: srv.URL + tc.prefix, APIKey: tc.key, Model: "claude-sonnet-4-6"})
			out, err := c.Complete(context.Background(), "user-1", tc.system, "USER", nil)
			if err != nil || out != `{"rules":[]}` {
				t.Fatalf("content = %q, err = %v", out, err)
			}
		})
	}
}

func TestComplete_AnthropicRejectsInvalidOrIncompleteReplies(t *testing.T) {
	for _, body := range []string{
		`not json sk-SECRET`,
		`{"content":[]}`,
		`{"choices":[{"message":{"content":"wrong API"}}]}`,
		`{"content":[{"type":"thinking","thinking":"sk-SECRET"}]}`,
		`{"content":[{"type":"text","text":"  "}]}`,
		`{"content":[{"type":"text","text":42}]}`,
		`{"content":[{"type":"text","text":"sk-SECRET"}],"stop_reason":"max_tokens"}`,
		`{"content":[{"type":"text","text":"sk-SECRET"}],"stop_reason":"refusal"}`,
		`{"content":[{"type":"text","text":"sk-SECRET"}],"stop_reason":"tool_use"}`,
		`{"content":[{"type":"text","text":"sk-SECRET"}],"stop_reason":"pause_turn"}`,
		`{"content":[{"type":"text","text":"sk-SECRET"}],"stop_reason":"model_context_window_exceeded"}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, body)
		}))
		out, err := New(Config{Dialect: "anthropic", Endpoint: srv.URL, APIKey: "sk-SECRET", Model: "m"}).Complete(context.Background(), "u", "s", "u", nil)
		srv.Close()
		if err == nil || out != "" {
			t.Fatalf("body %q: content=%q err=%v", body, out, err)
		}
		if strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("error leaks response body or key: %v", err)
		}
	}
}

func TestComplete_AnthropicDoesNotForwardKeyOnRedirect(t *testing.T) {
	forwarded := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = true
	}))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	_, err := New(Config{Dialect: "anthropic", Endpoint: srv.URL, APIKey: "sk-test", Model: "m"}).Complete(context.Background(), "u", "s", "u", nil)
	if forwarded || err == nil || !strings.Contains(err.Error(), "307") {
		t.Fatalf("forwarded=%v err=%v", forwarded, err)
	}
}

func TestComplete_ErrorsNeverCarryTheBodyOrKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key sk-test leaked"}}`)
	}))
	defer srv.Close()
	for _, dialect := range []string{"", "anthropic"} {
		_, err := New(Config{Dialect: dialect, Endpoint: srv.URL, APIKey: "sk-test", Model: "m"}).Complete(context.Background(), "user-1", "s", "u", nil)
		if err == nil {
			t.Fatal("expected an error on 401")
		}
		if strings.Contains(err.Error(), "sk-test") || strings.Contains(err.Error(), "leaked") {
			t.Fatalf("error leaks response body or key: %v", err)
		}
		if !strings.Contains(err.Error(), "401") {
			t.Fatalf("error should name the status: %v", err)
		}
	}
}

func TestComplete_MalformedAndEmptyReplies(t *testing.T) {
	for _, body := range []string{`not json`, `{"choices":[]}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, body)
		}))
		_, err := New(Config{Endpoint: srv.URL, Model: "m"}).Complete(context.Background(), "user-1", "s", "u", nil)
		srv.Close()
		if err == nil {
			t.Fatalf("body %q: expected an error", body)
		}
	}
}

func TestComplete_SignSuppliesThePerUserBearer(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()
	c := New(Config{Endpoint: srv.URL, APIKey: "ignored", Model: "m", Sign: func(uid string) (string, error) { return "tok-for-" + uid, nil }})
	if _, err := c.Complete(context.Background(), "user-7", "s", "u", nil); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer tok-for-user-7" {
		t.Fatalf("Authorization = %q", auth)
	}
}

func TestComplete_SignFailureSendsNothing(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	c := New(Config{Endpoint: srv.URL, Model: "m", Sign: func(string) (string, error) { return "", errors.New("boom") }})
	if _, err := c.Complete(context.Background(), "u", "s", "u", nil); err == nil || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

var testSchema = &Schema{Name: "rows", JSON: json.RawMessage(`{"type":"object","properties":{"rows":{"type":"array","items":{"type":"string"}}},"required":["rows"],"additionalProperties":false}`)}

func TestComplete_SendsTheSchemaInEachDialect(t *testing.T) {
	for _, dialect := range []string{"", "econumo", "anthropic"} {
		t.Run("dialect="+dialect, func(t *testing.T) {
			var body map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&body)
				if dialect == "anthropic" {
					_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"{}"}],"stop_reason":"end_turn"}`)
				} else {
					_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
				}
			}))
			defer srv.Close()
			if _, err := New(Config{Dialect: dialect, Endpoint: srv.URL, Model: "m", JSONSchema: true}).Complete(context.Background(), "u", "s", "u", testSchema); err != nil {
				t.Fatal(err)
			}
			var want, got any
			_ = json.Unmarshal(testSchema.JSON, &want)
			if dialect == "anthropic" {
				if _, has := body["response_format"]; has {
					t.Fatal("anthropic must not get response_format")
				}
				format, _ := body["output_config"].(map[string]any)["format"].(map[string]any)
				if format["type"] != "json_schema" {
					t.Fatalf("output_config = %v", body["output_config"])
				}
				got = format["schema"]
			} else {
				if _, has := body["output_config"]; has {
					t.Fatal("chat completions must not get output_config")
				}
				rf, _ := body["response_format"].(map[string]any)
				js, _ := rf["json_schema"].(map[string]any)
				if rf["type"] != "json_schema" || js["name"] != "rows" || js["strict"] != true {
					t.Fatalf("response_format = %v", rf)
				}
				got = js["schema"]
			}
			gb, _ := json.Marshal(got)
			wb, _ := json.Marshal(want)
			if string(gb) != string(wb) {
				t.Fatalf("schema = %s, want %s", gb, wb)
			}
		})
	}
}

func TestComplete_A400ToASchemaRetriesWithoutAndRemembers(t *testing.T) {
	for _, dialect := range []string{"", "anthropic"} {
		t.Run("dialect="+dialect, func(t *testing.T) {
			var withSchema, without int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				_, rf := body["response_format"]
				_, oc := body["output_config"]
				if rf || oc {
					withSchema++
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				without++
				if dialect == "anthropic" {
					_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`)
				} else {
					_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
				}
			}))
			defer srv.Close()
			c := New(Config{Dialect: dialect, Endpoint: srv.URL, Model: "m", JSONSchema: true})
			for i := 0; i < 2; i++ {
				out, err := c.Complete(context.Background(), "u", "s", "u", testSchema)
				if err != nil || out != "ok" {
					t.Fatalf("call %d: out=%q err=%v", i, out, err)
				}
			}
			if withSchema != 1 || without != 2 {
				t.Fatalf("with schema %d, without %d; want 1 and 2", withSchema, without)
			}
		})
	}
}

func TestComplete_A400WithoutASchemaIsNotRetried(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	c := New(Config{Endpoint: srv.URL, Model: "m"})
	if _, err := c.Complete(context.Background(), "u", "s", "u", nil); err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	// a 400 that is not about a schema must not turn schemas off
	if c.noSchema.Load() {
		t.Fatal("noSchema set by a schema-less 400")
	}
}

func TestComplete_OtherErrorsToASchemaKeepSchemas(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(Config{Endpoint: srv.URL, Model: "m", JSONSchema: true})
	if _, err := c.Complete(context.Background(), "u", "s", "u", testSchema); err == nil || calls != 1 || c.noSchema.Load() {
		t.Fatalf("err=%v calls=%d noSchema=%v", err, calls, c.noSchema.Load())
	}
}

func TestSchemaCheck(t *testing.T) {
	if err := testSchema.Check(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`{"type":"object","properties":{"a":{"type":"string"}},"required":["a"]}`,
		`{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"}},"required":["a"],"additionalProperties":false}`,
		`{"type":"object","properties":{"a":{"type":"array","items":{"type":"object","properties":{},"required":[]}}},"required":["a"],"additionalProperties":false}`,
		`{"type":"object","properties":{"a":{"type":"string","maxLength":3}},"required":["a"],"additionalProperties":false}`,
		`not json`,
	} {
		if err := (Schema{Name: "x", JSON: json.RawMessage(bad)}).Check(); err == nil {
			t.Fatalf("%s: expected an error", bad)
		}
	}
}

func TestComplete_WithoutTheJSONFlagNoSchemaIsSent(t *testing.T) {
	for _, dialect := range []string{"", "econumo", "anthropic"} {
		var body map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&body)
			if dialect == "anthropic" {
				_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"{}"}],"stop_reason":"end_turn"}`)
			} else {
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
			}
		}))
		_, err := New(Config{Dialect: dialect, Endpoint: srv.URL, Model: "m"}).Complete(context.Background(), "u", "s", "u", testSchema)
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"response_format", "output_config"} {
			if _, has := body[field]; has {
				t.Fatalf("dialect %q: %s sent without json=1", dialect, field)
			}
		}
	}
}

func TestComplete_NonSuccessIsAnHTTPErrorWithoutBody(t *testing.T) {
	for _, code := range []int{401, 429, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = io.WriteString(w, `secret-body sk-test`)
		}))
		for _, dialect := range []string{"", "anthropic"} {
			_, err := New(Config{Dialect: dialect, Endpoint: srv.URL, APIKey: "sk-test", Model: "m"}).Complete(context.Background(), "u", "s", "u", nil)
			var he *HTTPError
			if !errors.As(err, &he) || he.StatusCode != code {
				t.Fatalf("code %d dialect %q: err = %v", code, dialect, err)
			}
			if strings.Contains(err.Error(), "secret-body") || strings.Contains(err.Error(), "sk-test") {
				t.Fatalf("error leaks: %v", err)
			}
		}
		srv.Close()
	}
}

func TestComplete_UnusableRepliesAreResponseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `not json`)
	}))
	defer srv.Close()
	_, err := New(Config{Endpoint: srv.URL, Model: "m"}).Complete(context.Background(), "u", "s", "u", nil)
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("err = %v", err)
	}
}

func TestLogAttrs(t *testing.T) {
	got := LogAttrs(&HTTPError{StatusCode: 429, Dialect: "openai"})
	if len(got) != 4 || got[0] != "err_type" || got[1] != "*ai.HTTPError" || got[3] != 429 {
		t.Fatalf("attrs = %v", got)
	}
	got = LogAttrs(fmt.Errorf("ai: request failed: %w", context.DeadlineExceeded))
	if len(got) != 2 || got[1] == "*fmt.wrapError" {
		t.Fatalf("attrs = %v", got)
	}
}

// transportFailure calls an endpoint that gives no answer, with no proxy in
// the way, and returns the error and its log attributes.
func transportFailure(t *testing.T, ctx context.Context, endpoint string) (error, []any) {
	t.Helper()
	c := New(Config{Endpoint: endpoint, APIKey: "sk-SECRET", Model: "m"})
	c.http.Transport = &http.Transport{}
	_, err := c.Complete(ctx, "u", "s", "u", nil)
	var te *TransportError
	if !errors.As(err, &te) {
		t.Fatalf("err = %T %v", err, err)
	}
	if msg := err.Error(); strings.Contains(msg, "sk-SECRET") || strings.Contains(msg, endpoint) || strings.Contains(msg, "://") {
		t.Fatalf("error text leaks the endpoint: %q", msg)
	}
	return err, LogAttrs(err)
}

func TestLogAttrs_TransportFailureCause(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + ln.Addr().String() + "/v1?key=sk-SECRET"
	_ = ln.Close()
	if _, got := transportFailure(t, context.Background(), closed); got[1] != "*ai.TransportError" || got[3] != "connection_refused" {
		t.Fatalf("refused: attrs = %v", got)
	}

	if _, got := transportFailure(t, context.Background(), "http://sk-SECRET@econumo-test.invalid/v1"); got[3] != "dns" {
		t.Fatalf("dns: attrs = %v", got)
	}

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, got := transportFailure(t, ctx, srv.URL); got[3] != "timeout" || got[1] == "*ai.TransportError" {
		t.Fatalf("timeout: attrs = %v", got)
	}
}
