package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultDockerHubHost = "registry-1.docker.io"
	defaultAuthBaseURL   = "https://auth.docker.io"
)

var (
	defaultRoutes = map[string]string{
		"quay":       "quay.io",
		"gcr":        "gcr.io",
		"k8s-gcr":    "k8s.gcr.io",
		"k8s":        "registry.k8s.io",
		"ghcr":       "ghcr.io",
		"cloudsmith": "docker.cloudsmith.io",
		"nvcr":       "nvcr.io",
	}
	defaultBlockedUAs = []string{"netcraft"}
)

var (
	v2ShortPathRegex = regexp.MustCompile(`^/v2/[^/]+/[^/]+/[^/]+$`)
	v2LibraryRegex   = regexp.MustCompile(`^/v2/library`)
	repoExtractRegex = regexp.MustCompile(`^/v2/(.+?)(?:/(manifests|blobs|tags)/)`)
	repoExtractList  = regexp.MustCompile(`^/v2/(.+?)/tags/list`)
)

type healthCheck struct {
	Name string
	URL  string
}

type options struct {
	dockerHubHost  string
	authBaseURL    string
	upstreamScheme string
	browserHubHost string
	browserV1Host  string
	routes         map[string]string
	blockedUAs     []string
	registryClient *http.Client
	downloadClient *http.Client
	healthChecks   []healthCheck
	listenLabel    string
}

type tokenEntry struct {
	token   string
	expires time.Time
}

type app struct {
	opts         options
	tokenCacheMu sync.RWMutex
	tokenCache   map[string]tokenEntry
}

func defaultOptions() options {
	return options{
		dockerHubHost:  defaultDockerHubHost,
		authBaseURL:    defaultAuthBaseURL,
		upstreamScheme: "https",
		browserHubHost: "hub.docker.com",
		browserV1Host:  "index.docker.io",
		routes:         cloneMap(defaultRoutes),
		blockedUAs:     append([]string(nil), defaultBlockedUAs...),
		registryClient: &http.Client{
			Timeout: 300 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:       &tls.Config{},
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   50,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		downloadClient: &http.Client{
			Timeout: 600 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:       &tls.Config{},
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   50,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: 120 * time.Second,
			},
		},
		healthChecks: []healthCheck{
			{
				Name: "auth.docker.io",
				URL:  "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/alpine:pull",
			},
			{
				Name: "registry-1.docker.io",
				URL:  "https://registry-1.docker.io/v2/",
			},
			{
				Name: "hub.docker.com",
				URL:  "https://hub.docker.com/",
			},
		},
		listenLabel: "serverless",
	}
}

func newApp(opts options) *app {
	if opts.dockerHubHost == "" {
		opts.dockerHubHost = defaultDockerHubHost
	}
	if opts.authBaseURL == "" {
		opts.authBaseURL = defaultAuthBaseURL
	}
	if opts.upstreamScheme == "" {
		opts.upstreamScheme = "https"
	}
	if opts.browserHubHost == "" {
		opts.browserHubHost = "hub.docker.com"
	}
	if opts.browserV1Host == "" {
		opts.browserV1Host = "index.docker.io"
	}
	if len(opts.routes) == 0 {
		opts.routes = cloneMap(defaultRoutes)
	} else {
		opts.routes = cloneMap(opts.routes)
	}
	if len(opts.blockedUAs) == 0 {
		opts.blockedUAs = append([]string(nil), defaultBlockedUAs...)
	} else {
		opts.blockedUAs = append([]string(nil), opts.blockedUAs...)
	}
	if opts.registryClient == nil {
		opts.registryClient = defaultOptions().registryClient
	}
	if opts.downloadClient == nil {
		opts.downloadClient = defaultOptions().downloadClient
	}
	if len(opts.healthChecks) == 0 {
		opts.healthChecks = append([]healthCheck(nil), defaultOptions().healthChecks...)
	} else {
		opts.healthChecks = append([]healthCheck(nil), opts.healthChecks...)
	}
	if opts.listenLabel == "" {
		opts.listenLabel = "serverless"
	}

	return &app{
		opts:       opts,
		tokenCache: make(map[string]tokenEntry),
	}
}

// NewHandler 返回共享的 HTTP 入口，供 EdgeOne Pages 与 Vercel 复用。
func NewHandler() http.Handler {
	return newApp(defaultOptions())
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Fatal(http.ListenAndServe(":"+port, NewHandler()))
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[%s] %s %s%s", r.RemoteAddr, r.Method, r.URL.Path, queryString(r))

	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,TRACE,DELETE,HEAD,OPTIONS")
		w.Header().Set("Access-Control-Max-Age", "1728000")
		w.WriteHeader(http.StatusOK)
		return
	}

	hubHost, isDockerHub := a.resolveUpstream(r)
	userAgent := strings.ToLower(r.Header.Get("User-Agent"))

	if a.isBlockedUA(userAgent) {
		serveNginxPage(w)
		return
	}

	path := r.URL.Path
	isBrowser := strings.Contains(userAgent, "mozilla")
	isV1Hub := strings.Contains(path, "/v1/search") || strings.Contains(path, "/v1/repositories")

	if isBrowser || isV1Hub {
		a.handleBrowser(w, r, hubHost, isDockerHub)
		return
	}

	switch {
	case path == "/v2/" || path == "/v2":
		handleV2Ping(w)
	case strings.Contains(path, "/token"):
		a.handleToken(w, r)
	case strings.HasPrefix(path, "/v2/"):
		a.handleV2(w, r, hubHost, isDockerHub)
	case path == "/health":
		a.handleHealth(w)
	default:
		a.proxyDirect(w, r, hubHost)
	}
}

func (a *app) resolveUpstream(r *http.Request) (hubHost string, isDockerHub bool) {
	if ns := r.URL.Query().Get("ns"); ns != "" {
		if ns == "docker.io" {
			return a.opts.dockerHubHost, true
		}
		return ns, false
	}

	hostname := r.URL.Query().Get("hubhost")
	if hostname == "" {
		hostname = r.Host
	}
	hostTop := strings.Split(hostname, ".")[0]
	if upstream, ok := a.opts.routes[hostTop]; ok {
		return upstream, false
	}
	return a.opts.dockerHubHost, true
}

func (a *app) isBlockedUA(userAgent string) bool {
	for _, blocked := range a.opts.blockedUAs {
		if strings.Contains(userAgent, strings.ToLower(blocked)) {
			return true
		}
	}
	return false
}

func (a *app) handleBrowser(w http.ResponseWriter, r *http.Request, hubHost string, isDockerHub bool) {
	path := r.URL.Path

	if path == "/" {
		if isDockerHub {
			serveSearchPage(w)
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintf(w, "Docker Registry Proxy → %s", hubHost)
		}
		return
	}

	if strings.HasPrefix(path, "/v1/") {
		a.proxyBrowser(w, r, a.opts.browserV1Host)
		return
	}

	if isDockerHub {
		if q := r.URL.Query().Get("q"); strings.Contains(q, "library/") && q != "library/" {
			values := r.URL.Query()
			values.Set("q", strings.Replace(q, "library/", "", 1))
			r = cloneRequest(r)
			r.URL.RawQuery = values.Encode()
		}
		a.proxyBrowser(w, r, a.opts.browserHubHost)
		return
	}

	a.proxyBrowser(w, r, hubHost)
}

func (a *app) proxyBrowser(w http.ResponseWriter, r *http.Request, host string) {
	target := buildURL(a.opts.upstreamScheme, host, r.URL.Path, r.URL.RawQuery)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	copyAllHeaders(req.Header, r.Header)
	req.Host = host

	resp, err := a.opts.downloadClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	flushResponse(w, resp)
}

func (a *app) handleHealth(w http.ResponseWriter) {
	type checkResult struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Status  string `json:"status"`
		Latency string `json:"latency"`
		Detail  string `json:"detail,omitempty"`
	}

	results := make([]checkResult, 0, len(a.opts.healthChecks))
	for _, check := range a.opts.healthChecks {
		start := time.Now()
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, check.URL, nil)
		if err != nil {
			results = append(results, checkResult{
				Name:    check.Name,
				URL:     check.URL,
				Status:  "FAIL",
				Latency: "0s",
				Detail:  err.Error(),
			})
			continue
		}
		req.Header.Set("User-Agent", "docker-proxy/health-check")

		resp, err := a.opts.registryClient.Do(req)
		elapsed := time.Since(start).Round(time.Millisecond)

		result := checkResult{
			Name:    check.Name,
			URL:     check.URL,
			Latency: elapsed.String(),
		}
		if err != nil {
			result.Status = "FAIL"
			result.Detail = err.Error()
		} else {
			resp.Body.Close()
			result.Status = fmt.Sprintf("HTTP %d", resp.StatusCode)
			if resp.StatusCode < 500 {
				result.Detail = "OK"
			}
		}
		results = append(results, result)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(map[string]any{
		"proxy":  "running",
		"time":   time.Now().Format(time.RFC3339),
		"listen": a.opts.listenLabel,
		"checks": results,
	})
}

func handleV2Ping(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

func (a *app) handleToken(w http.ResponseWriter, r *http.Request) {
	authBaseURL, err := url.Parse(a.opts.authBaseURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	target := a.opts.authBaseURL + r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Host = authBaseURL.Host
	copySelectHeaders(req.Header, r.Header)

	resp, err := a.opts.registryClient.Do(req)
	if err != nil {
		log.Printf("token 代理失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	flushResponse(w, resp)
}

func (a *app) handleV2(w http.ResponseWriter, r *http.Request, hubHost string, isDockerHub bool) {
	path := r.URL.Path
	rawQuery := r.URL.RawQuery

	if isDockerHub && !containsCI(rawQuery, "%2F") {
		fullURI := path
		if rawQuery != "" {
			fullURI += "?" + rawQuery
		}
		if containsCI(fullURI, "%3A") {
			if fixed := fixEncodedLibrary(fullURI); fixed != fullURI {
				if queryIndex := strings.Index(fixed, "?"); queryIndex != -1 {
					path = fixed[:queryIndex]
					rawQuery = fixed[queryIndex+1:]
				} else {
					path = fixed
					rawQuery = ""
				}
				log.Printf("编码修正: %s -> %s", r.URL.RequestURI(), fixed)
			}
		}
	}

	if isDockerHub && v2ShortPathRegex.MatchString(path) && !v2LibraryRegex.MatchString(path) {
		if parts := strings.SplitN(path, "/v2/", 2); len(parts) == 2 {
			path = "/v2/library/" + parts[1]
			log.Printf("补全 library/: %s -> %s", r.URL.Path, path)
		}
	}

	needsAuth := strings.Contains(path, "/manifests/") ||
		strings.Contains(path, "/blobs/") ||
		strings.Contains(path, "/tags/")

	var token string
	if needsAuth {
		if repo := extractRepo(path); repo != "" {
			var err error
			token, err = a.getToken(repo)
			if err != nil {
				log.Printf("获取 token 失败 (repo=%s): %v", repo, err)
			}
		}
	}

	target := buildURL(a.opts.upstreamScheme, hubHost, path, rawQuery)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	copySelectHeaders(req.Header, r.Header)
	req.Host = hubHost
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if auth := r.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if value := r.Header.Get("X-Amz-Content-Sha256"); value != "" {
		req.Header.Set("X-Amz-Content-Sha256", value)
	}

	resp, err := a.opts.registryClient.Do(req)
	if err != nil {
		log.Printf("上游请求失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if location := resp.Header.Get("Location"); location != "" && isRedirectCode(resp.StatusCode) {
		log.Printf("跟随重定向: %s", location)
		a.handleCDNRedirect(w, r, location)
		return
	}

	if resp.StatusCode == http.StatusUnauthorized {
		log.Printf("上游 401 (path=%s)", path)
	}

	flushResponse(w, resp)
}

func (a *app) handleCDNRedirect(w http.ResponseWriter, original *http.Request, location string) {
	req, err := http.NewRequestWithContext(original.Context(), original.Method, location, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	copyAllHeaders(req.Header, original.Header)
	req.Header.Del("Authorization")

	resp, err := a.opts.downloadClient.Do(req)
	if err != nil {
		log.Printf("CDN 下载失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("Access-Control-Expose-Headers", "*")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "max-age=1500")
	w.Header().Del("Content-Security-Policy")
	w.Header().Del("Content-Security-Policy-Report-Only")
	w.Header().Del("Clear-Site-Data")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (a *app) proxyDirect(w http.ResponseWriter, r *http.Request, hubHost string) {
	target := buildURL(a.opts.upstreamScheme, hubHost, r.URL.Path, r.URL.RawQuery)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	copySelectHeaders(req.Header, r.Header)
	req.Host = hubHost
	if auth := r.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if value := r.Header.Get("X-Amz-Content-Sha256"); value != "" {
		req.Header.Set("X-Amz-Content-Sha256", value)
	}

	resp, err := a.opts.registryClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if location := resp.Header.Get("Location"); location != "" && isRedirectCode(resp.StatusCode) {
		a.handleCDNRedirect(w, r, location)
		return
	}

	flushResponse(w, resp)
}

func (a *app) getCachedToken(repo string) (string, bool) {
	a.tokenCacheMu.RLock()
	defer a.tokenCacheMu.RUnlock()

	if entry, ok := a.tokenCache[repo]; ok && time.Now().Before(entry.expires) {
		return entry.token, true
	}
	return "", false
}

func (a *app) setCachedToken(repo, token string, ttl time.Duration) {
	a.tokenCacheMu.Lock()
	defer a.tokenCacheMu.Unlock()

	a.tokenCache[repo] = tokenEntry{
		token:   token,
		expires: time.Now().Add(ttl),
	}
}

func (a *app) getToken(repo string) (string, error) {
	if token, ok := a.getCachedToken(repo); ok {
		return token, nil
	}

	tokenURL := fmt.Sprintf("%s/token?service=registry.docker.io&scope=repository:%s:pull", strings.TrimRight(a.opts.authBaseURL, "/"), repo)
	req, err := http.NewRequest(http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "docker-proxy/1.0")

	resp, err := a.opts.registryClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 token 失败: %w", err)
	}

	var result struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析 token 失败: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("token 为空, resp=%s", string(body))
	}

	ttl := time.Duration(result.ExpiresIn) * time.Second
	if ttl <= 0 || ttl > 300*time.Second {
		ttl = 250 * time.Second
	} else {
		ttl -= 30 * time.Second
	}
	a.setCachedToken(repo, result.Token, ttl)
	log.Printf("token 已缓存 (repo=%s, ttl=%s)", repo, ttl)
	return result.Token, nil
}

func extractRepo(path string) string {
	if matches := repoExtractRegex.FindStringSubmatch(path); len(matches) > 1 {
		return matches[1]
	}
	if matches := repoExtractList.FindStringSubmatch(path); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func copySelectHeaders(dst, src http.Header) {
	for _, key := range []string{
		"User-Agent", "Accept", "Accept-Language", "Accept-Encoding",
		"Connection", "Cache-Control", "If-None-Match", "If-Modified-Since",
	} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}

func copyAllHeaders(dst, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "Host") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func flushResponse(w http.ResponseWriter, resp *http.Response) {
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "*")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func isRedirectCode(code int) bool {
	return code == http.StatusMovedPermanently ||
		code == http.StatusFound ||
		code == http.StatusSeeOther ||
		code == http.StatusTemporaryRedirect ||
		code == http.StatusPermanentRedirect
}

func fixEncodedLibrary(uri string) string {
	lowerURI := strings.ToLower(uri)
	index := strings.Index(lowerURI, "%3a")
	if index == -1 {
		return uri
	}
	rest := uri[index+3:]
	if strings.Contains(rest, "&") {
		return uri[:index+3] + "library%2F" + rest
	}
	return uri
}

func containsCI(source, target string) bool {
	return strings.Contains(strings.ToLower(source), strings.ToLower(target))
}

func queryString(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}
	return "?" + r.URL.RawQuery
}

func buildURL(scheme, host, path, rawQuery string) string {
	target := fmt.Sprintf("%s://%s%s", scheme, host, path)
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	return target
}

func cloneMap(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func cloneRequest(r *http.Request) *http.Request {
	cloned := r.Clone(r.Context())
	cloned.URL = new(url.URL)
	*cloned.URL = *r.URL
	return cloned
}

func serveNginxPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Welcome to nginx!</title>
<style>body{width:35em;margin:0 auto;font-family:Tahoma,Verdana,Arial,sans-serif;}</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and working. Further configuration is required.</p>
<p>For online documentation and support please refer to <a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at <a href="http://nginx.com/">nginx.com</a>.</p>
<p><em>Thank you for using nginx.</em></p>
</body></html>`)
}

func serveSearchPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
	<title>Docker Hub 镜像搜索</title>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
	:root {
		--primary-color: #0066ff;
		--primary-dark: #0052cc;
		--gradient-start: #1a90ff;
		--gradient-end: #003eb3;
		--text-color: #ffffff;
		--transition-time: 0.3s;
	}
	* { box-sizing: border-box; margin: 0; padding: 0; }
	body {
		font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
		display: flex; flex-direction: column; justify-content: center; align-items: center;
		min-height: 100vh; margin: 0;
		background: linear-gradient(135deg, var(--gradient-start) 0%, var(--gradient-end) 100%);
		padding: 20px; color: var(--text-color); overflow-x: hidden;
	}
	.container {
		text-align: center; width: 100%; max-width: 800px; padding: 20px; margin: 0 auto;
		display: flex; flex-direction: column; justify-content: center; min-height: 60vh;
		animation: fadeIn 0.8s ease-out;
	}
	@keyframes fadeIn { from { opacity: 0; transform: translateY(20px); } to { opacity: 1; transform: translateY(0); } }
	.logo { margin-bottom: 20px; animation: float 6s ease-in-out infinite; }
	@keyframes float { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-10px); } }
	.logo:hover { transform: scale(1.08) rotate(5deg); }
	.logo svg { filter: drop-shadow(0 5px 15px rgba(0,0,0,0.2)); }
	.title {
		color: var(--text-color); font-size: 2.3em; margin-bottom: 10px;
		text-shadow: 0 2px 10px rgba(0,0,0,0.2); font-weight: 700; letter-spacing: -0.5px;
	}
	.subtitle {
		color: rgba(255,255,255,0.9); font-size: 1.1em; margin-bottom: 25px;
		max-width: 600px; margin-left: auto; margin-right: auto; line-height: 1.4;
	}
	.search-container {
		display: flex; align-items: stretch; width: 100%; max-width: 600px; margin: 0 auto;
		height: 55px; box-shadow: 0 10px 25px rgba(0,0,0,0.15); border-radius: 12px; overflow: hidden;
	}
	#search-input {
		flex: 1; padding: 0 20px; font-size: 16px; border: none; outline: none;
		transition: all var(--transition-time) ease; height: 100%;
	}
	#search-input:focus { padding-left: 25px; }
	#search-button {
		width: 60px; background-color: var(--primary-color); border: none; cursor: pointer;
		transition: all var(--transition-time) ease; height: 100%;
		display: flex; align-items: center; justify-content: center;
	}
	#search-button svg { transition: transform 0.3s ease; stroke: white; }
	#search-button:hover { background-color: var(--primary-dark); }
	#search-button:hover svg { transform: translateX(2px); }
	.tips { color: rgba(255,255,255,0.8); margin-top: 20px; font-size: 0.9em; }
	@media (max-width: 768px) { .title { font-size: 2em; } .search-container { height: 50px; } }
	@media (max-width: 480px) { .title { font-size: 1.7em; } .search-container { height: 45px; } #search-button { width: 50px; } }
	</style>
</head>
<body>
	<div class="container">
		<div class="logo">
			<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 18" fill="#ffffff" width="110" height="85">
				<path d="M23.763 6.886c-.065-.053-.673-.512-1.954-.512-.32 0-.659.03-1.01.087-.248-1.703-1.651-2.533-1.716-2.57l-.345-.2-.227.328a4.596 4.596 0 0 0-.611 1.433c-.23.972-.09 1.884.403 2.666-.596.331-1.546.418-1.744.42H.752a.753.753 0 0 0-.75.749c-.007 1.456.233 2.864.692 4.07.545 1.43 1.355 2.483 2.409 3.13 1.181.725 3.104 1.14 5.276 1.14 1.016 0 2.03-.092 2.93-.266 1.417-.273 2.705-.742 3.826-1.391a10.497 10.497 0 0 0 2.61-2.14c1.252-1.42 1.998-3.005 2.553-4.408.075.003.148.005.221.005 1.371 0 2.215-.55 2.68-1.01.505-.5.685-.998.704-1.053L24 7.076l-.237-.19Z"></path>
				<path d="M2.216 8.075h2.119a.186.186 0 0 0 .185-.186V6a.186.186 0 0 0-.185-.186H2.216A.186.186 0 0 0 2.031 6v1.89c0 .103.083.186.185.186Zm2.92 0h2.118a.185.185 0 0 0 .185-.186V6a.185.185 0 0 0-.185-.186H5.136A.185.185 0 0 0 4.95 6v1.89c0 .103.083.186.186.186Zm2.964 0h2.118a.186.186 0 0 0 .185-.186V6a.186.186 0 0 0-.185-.186H8.1A.185.185 0 0 0 7.914 6v1.89c0 .103.083.186.186.186Zm2.928 0h2.119a.185.185 0 0 0 .185-.186V6a.185.185 0 0 0-.185-.186h-2.119a.186.186 0 0 0-.185.186v1.89c0 .103.083.186.185.186Zm-5.892-2.72h2.118a.185.185 0 0 0 .185-.186V3.28a.186.186 0 0 0-.185-.186H5.136a.186.186 0 0 0-.186.186v1.89c0 .103.083.186.186.186Zm2.964 0h2.118a.186.186 0 0 0 .185-.186V3.28a.186.186 0 0 0-.185-.186H8.1a.186.186 0 0 0-.186.186v1.89c0 .103.083.186.186.186Zm2.928 0h2.119a.185.185 0 0 0 .185-.186V3.28a.185.185 0 0 0-.185-.186h-2.119a.186.186 0 0 0-.185.186v1.89c0 .103.083.186.185.186Zm0-2.72h2.119a.186.186 0 0 0 .185-.186V.56a.185.185 0 0 0-.185-.186h-2.119a.186.186 0 0 0-.185.186v1.89c0 .103.083.186.185.186Zm2.955 5.44h2.118a.185.185 0 0 0 .186-.186V6a.185.185 0 0 0-.186-.186h-2.118a.185.185 0 0 0-.185.186v1.89c0 .103.083.186.185.186Z"></path>
			</svg>
		</div>
		<h1 class="title">Docker Hub 镜像搜索</h1>
		<p class="subtitle">快速查找、下载和部署 Docker 容器镜像</p>
		<div class="search-container">
			<input type="text" id="search-input" placeholder="输入关键词搜索镜像，如: nginx, mysql, redis...">
			<button id="search-button" title="搜索">
				<svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M13 5l7 7-7 7M5 5l7 7-7 7" stroke-linecap="round" stroke-linejoin="round"></path>
				</svg>
			</button>
		</div>
		<p class="tips">Docker Registry Proxy — 自建镜像代理服务</p>
	</div>
	<script>
	function performSearch() {
		const q = document.getElementById('search-input').value;
		if (q) window.location.href = '/search?q=' + encodeURIComponent(q);
	}
	document.getElementById('search-button').addEventListener('click', performSearch);
	document.getElementById('search-input').addEventListener('keypress', function(e) {
		if (e.key === 'Enter') performSearch();
	});
	window.addEventListener('load', function() { document.getElementById('search-input').focus(); });
	</script>
</body>
</html>`)
}
