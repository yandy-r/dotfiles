package backend

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// redirectAPI points the request-level httpDo at srv while preserving the
// request path (so the api.github.com URL routing is exercised end-to-end).
func redirectAPI(t *testing.T, srv *httptest.Server) {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	orig := httpDo
	httpDo = func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		return srv.Client().Do(req)
	}
	t.Cleanup(func() { httpDo = orig })
}

func TestListReleaseAssets(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"assets":[{"name":"tool_Linux_x86_64.tar.gz"},{"name":"checksums.txt"}]}`))
	}))
	defer srv.Close()

	redirectAPI(t, srv)
	t.Setenv("GITHUB_TOKEN", "secret-token")
	t.Setenv("GH_TOKEN", "")

	names, err := listReleaseAssets("owner/tool", "v1.2.3")
	if err != nil {
		t.Fatalf("listReleaseAssets() error = %v, want nil", err)
	}
	if len(names) != 2 || names[0] != "tool_Linux_x86_64.tar.gz" || names[1] != "checksums.txt" {
		t.Errorf("listReleaseAssets() = %v, want [tool_Linux_x86_64.tar.gz checksums.txt]", names)
	}
	if gotPath != "/repos/owner/tool/releases/tags/v1.2.3" {
		t.Errorf("request path = %q, want /repos/owner/tool/releases/tags/v1.2.3", gotPath)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q, want Bearer secret-token", gotAuth)
	}
}

func TestListReleaseAssets_MissingTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	redirectAPI(t, srv)

	if _, err := listReleaseAssets("owner/tool", "v9.9.9"); err == nil {
		t.Fatal("listReleaseAssets() error = nil, want non-200 error")
	}
}

func TestChecksumFor(t *testing.T) {
	sidecar := "abc123def4567890abc123def4567890abc123def4567890abc123def4567890  tool_Linux_x86_64.tar.gz\n" +
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff *tool_Darwin_arm64.tar.gz\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sidecar))
	}))
	defer srv.Close()

	orig := httpGet
	httpGet = func(_ string) (*http.Response, error) { return srv.Client().Get(srv.URL + "/checksums.txt") }
	t.Cleanup(func() { httpGet = orig })

	t.Run("space-space line", func(t *testing.T) {
		got, err := checksumFor("owner/tool", "v1", "checksums.txt", "tool_Linux_x86_64.tar.gz")
		if err != nil {
			t.Fatalf("checksumFor() error = %v", err)
		}
		if got != "abc123def4567890abc123def4567890abc123def4567890abc123def4567890" {
			t.Errorf("checksumFor() = %q, want the linux sha", got)
		}
	})

	t.Run("binary-mode marker line", func(t *testing.T) {
		got, err := checksumFor("owner/tool", "v1", "checksums.txt", "tool_Darwin_arm64.tar.gz")
		if err != nil {
			t.Fatalf("checksumFor() error = %v", err)
		}
		if got != "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff" {
			t.Errorf("checksumFor() = %q, want the darwin sha", got)
		}
	})

	t.Run("absent asset line errors", func(t *testing.T) {
		_, err := checksumFor("owner/tool", "v1", "checksums.txt", "tool_Windows_x86_64.zip")
		if err == nil || !strings.Contains(err.Error(), "no line for asset") {
			t.Fatalf("checksumFor() error = %v, want absent-line error", err)
		}
	})
}

func TestChecksumFor_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	orig := httpGet
	httpGet = func(_ string) (*http.Response, error) { return srv.Client().Get(srv.URL + "/missing") }
	t.Cleanup(func() { httpGet = orig })

	if _, err := checksumFor("owner/tool", "v1", "checksums.txt", "tool.tar.gz"); err == nil {
		t.Fatal("checksumFor() error = nil, want non-200 error")
	}
}
