package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRootBrowserPage(t *testing.T) {
	handler := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "Docker Hub 镜像搜索") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBlockedUAReturnsNginxPage(t *testing.T) {
	handler := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/anything", nil)
	req.Header.Set("User-Agent", "Netcraft")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Welcome to nginx!") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestManifestRequestAddsLibraryPrefixAndTokenCache(t *testing.T) {
	var authCalls int32
	var registryCalls int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			atomic.AddInt32(&authCalls, 1)
			if got := r.URL.Query().Get("scope"); got != "repository:library/nginx:pull" {
				t.Fatalf("unexpected token scope: %s", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"test-token","expires_in":120}`))
		case r.URL.Path == "/v2/library/nginx/manifests/latest":
			atomic.AddInt32(&registryCalls, 1)
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("unexpected authorization: %s", got)
			}
			w.Header().Set("Docker-Content-Digest", "sha256:test")
			_, _ = w.Write([]byte(`{"schemaVersion":2}`))
		default:
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	registryClient, downloadClient := newTestClients(upstream)
	handler := newApp(options{
		dockerHubHost:  upstreamURL.Host,
		authBaseURL:    upstream.URL,
		upstreamScheme: upstreamURL.Scheme,
		browserHubHost: upstreamURL.Host,
		browserV1Host:  upstreamURL.Host,
		registryClient: registryClient,
		downloadClient: downloadClient,
		healthChecks:   []healthCheck{},
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/v2/nginx/manifests/latest", nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), `"schemaVersion":2`) {
			t.Fatalf("unexpected body: %s", recorder.Body.String())
		}
	}

	if got := atomic.LoadInt32(&authCalls); got != 1 {
		t.Fatalf("expected token to be fetched once, got %d", got)
	}
	if got := atomic.LoadInt32(&registryCalls); got != 2 {
		t.Fatalf("expected registry calls to be 2, got %d", got)
	}
}

func TestBlobRedirectFollowStripsAuthorization(t *testing.T) {
	var cdnAuthorization string

	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"blob-token","expires_in":120}`))
		case r.URL.Path == "/v2/library/alpine/blobs/sha256:test":
			w.Header().Set("Location", upstream.URL+"/cdn/blob")
			w.WriteHeader(http.StatusTemporaryRedirect)
		case r.URL.Path == "/cdn/blob":
			cdnAuthorization = r.Header.Get("Authorization")
			_, _ = w.Write([]byte("blob-data"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	registryClient, downloadClient := newTestClients(upstream)
	handler := newApp(options{
		dockerHubHost:  upstreamURL.Host,
		authBaseURL:    upstream.URL,
		upstreamScheme: upstreamURL.Scheme,
		browserHubHost: upstreamURL.Host,
		browserV1Host:  upstreamURL.Host,
		registryClient: registryClient,
		downloadClient: downloadClient,
		healthChecks:   []healthCheck{},
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/v2/library/alpine/blobs/sha256:test", nil)
	req.Header.Set("Authorization", "Bearer original-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if recorder.Body.String() != "blob-data" {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
	if cdnAuthorization != "" {
		t.Fatalf("authorization header should be stripped, got %q", cdnAuthorization)
	}
}

func TestTokenProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("scope"); got != "repository:library/alpine:pull" {
			t.Fatalf("unexpected scope: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"proxied-token"}`))
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	registryClient, downloadClient := newTestClients(upstream)
	handler := newApp(options{
		authBaseURL:    upstream.URL,
		upstreamScheme: upstreamURL.Scheme,
		dockerHubHost:  upstreamURL.Host,
		browserHubHost: upstreamURL.Host,
		browserV1Host:  upstreamURL.Host,
		registryClient: registryClient,
		downloadClient: downloadClient,
		healthChecks:   []healthCheck{},
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/token?scope=repository:library/alpine:pull", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "proxied-token") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestHealthAndRouting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"custom-token","expires_in":120}`))
		case "/health-auth":
			w.WriteHeader(http.StatusOK)
		case "/health-registry":
			w.WriteHeader(http.StatusUnauthorized)
		case "/health-browser":
			w.WriteHeader(http.StatusOK)
		case "/v2/custom/image/manifests/latest":
			if r.Host == "" {
				t.Fatal("expected host to be set")
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	registryClient, downloadClient := newTestClients(upstream)
	handler := newApp(options{
		dockerHubHost:  upstreamURL.Host,
		authBaseURL:    upstream.URL,
		upstreamScheme: upstreamURL.Scheme,
		browserHubHost: upstreamURL.Host,
		browserV1Host:  upstreamURL.Host,
		routes: map[string]string{
			"ghcr": upstreamURL.Host,
		},
		registryClient: registryClient,
		downloadClient: downloadClient,
		healthChecks: []healthCheck{
			{Name: "auth", URL: upstream.URL + "/health-auth"},
			{Name: "registry", URL: upstream.URL + "/health-registry"},
			{Name: "browser", URL: upstream.URL + "/health-browser"},
		},
		listenLabel: "serverless",
	})

	healthReq := httptest.NewRequest(http.MethodGet, "http://example.com/health", nil)
	healthRecorder := httptest.NewRecorder()
	handler.ServeHTTP(healthRecorder, healthReq)

	if healthRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected health status: %d", healthRecorder.Code)
	}
	var payload struct {
		Listen string `json:"listen"`
		Checks []struct {
			Status string `json:"status"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(healthRecorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode health payload: %v", err)
	}
	if payload.Listen != "serverless" || len(payload.Checks) != 3 {
		t.Fatalf("unexpected health payload: %+v", payload)
	}

	upstreamReq := httptest.NewRequest(http.MethodGet, "http://ghcr.example.com/v2/custom/image/manifests/latest?ns="+url.QueryEscape(upstreamURL.Host), nil)
	upstreamRecorder := httptest.NewRecorder()
	handler.ServeHTTP(upstreamRecorder, upstreamReq)

	if upstreamRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected upstream status: %d", upstreamRecorder.Code)
	}
	if got := strings.TrimSpace(upstreamRecorder.Body.String()); got != `{"ok":true}` {
		t.Fatalf("unexpected upstream body: %s", got)
	}
}

func TestHelpers(t *testing.T) {
	if got := extractRepo("/v2/library/alpine/tags/list"); got != "library/alpine" {
		t.Fatalf("unexpected repo: %s", got)
	}
	if got := fixEncodedLibrary("/v2/nginx/manifests/latest?foo%3Abar&x=1"); got != "/v2/nginx/manifests/latest?foo%3Alibrary%2Fbar&x=1" {
		t.Fatalf("unexpected fixed uri: %s", got)
	}
	if !containsCI("Hello%3AWorld", "%3a") {
		t.Fatal("expected case-insensitive match")
	}
}

func TestSearchProxyRewritesLibraryQuery(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "nginx" {
			t.Fatalf("unexpected q: %s", got)
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	registryClient, downloadClient := newTestClients(upstream)
	handler := newApp(options{
		dockerHubHost:  upstreamURL.Host,
		authBaseURL:    upstream.URL,
		upstreamScheme: upstreamURL.Scheme,
		browserHubHost: upstreamURL.Host,
		browserV1Host:  upstreamURL.Host,
		registryClient: registryClient,
		downloadClient: downloadClient,
		healthChecks:   []healthCheck{},
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/search?q=library/nginx", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != "ok" {
		t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func newTestClients(server *httptest.Server) (*http.Client, *http.Client) {
	registryClient := server.Client()
	registryClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return registryClient, server.Client()
}
