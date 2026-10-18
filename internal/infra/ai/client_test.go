package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	out, err := c.Complete(context.Background(), "user-1", "SYS", "USER")
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
		t.Fatal("response_format must not be sent (local servers reject it)")
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
	if _, err := New(Config{Endpoint: srv.URL, Model: "llama3"}).Complete(context.Background(), "user-1", "s", "u"); err != nil {
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
				for _, field := range []string{"temperature", "response_format", "tools", "stream"} {
					if _, has := body[field]; has {
						t.Errorf("unexpected %s", field)
					}
				}
				_, _ = io.WriteString(w, `{"content":[{"type":"thinking","thinking":"private","text":"ignore"},{"type":"text","text":"{\"rules\":"},{"type":"text","text":"[]}"}],"stop_reason":"end_turn"}`)
			}))
			defer srv.Close()
			c := New(Config{Dialect: "anthropic", Endpoint: srv.URL + tc.prefix, APIKey: tc.key, Model: "claude-sonnet-4-6"})
			out, err := c.Complete(context.Background(), "user-1", tc.system, "USER")
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
		out, err := New(Config{Dialect: "anthropic", Endpoint: srv.URL, APIKey: "sk-SECRET", Model: "m"}).Complete(context.Background(), "u", "s", "u")
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
	_, err := New(Config{Dialect: "anthropic", Endpoint: srv.URL, APIKey: "sk-test", Model: "m"}).Complete(context.Background(), "u", "s", "u")
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
		_, err := New(Config{Dialect: dialect, Endpoint: srv.URL, APIKey: "sk-test", Model: "m"}).Complete(context.Background(), "user-1", "s", "u")
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
		_, err := New(Config{Endpoint: srv.URL, Model: "m"}).Complete(context.Background(), "user-1", "s", "u")
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
	if _, err := c.Complete(context.Background(), "user-7", "s", "u"); err != nil {
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
	if _, err := c.Complete(context.Background(), "u", "s", "u"); err == nil || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}
