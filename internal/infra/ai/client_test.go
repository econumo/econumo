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

func TestComplete_ErrorsNeverCarryTheBodyOrKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key sk-test leaked"}}`)
	}))
	defer srv.Close()
	_, err := New(Config{Endpoint: srv.URL, APIKey: "sk-test", Model: "m"}).Complete(context.Background(), "user-1", "s", "u")
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
