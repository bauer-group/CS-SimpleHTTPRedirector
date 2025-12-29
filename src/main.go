package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RedirectRule defines a single redirect configuration
type RedirectRule struct {
	Source        []string `json:"source"`         // e.g., ["alte-domain.de", "www.alte-domain.de"]
	Target        string   `json:"target"`         // e.g., "https://neue-domain.de"
	Type          string   `json:"type"`           // "permanent", "temporary", or HTTP status code (301, 302, 307, 308)
	PreservePath  bool     `json:"preserve_path"`  // Keep the original path
	PreserveQuery bool     `json:"preserve_query"` // Keep query parameters
	statusCode    int      // Parsed status code (internal)
	parsedTarget  *url.URL // Parsed and validated target URL (internal)
}

// Config holds server configuration
type Config struct {
	TrustProxy bool // Trust X-Forwarded-* headers
}

// RateLimiter implements a simple token bucket rate limiter per IP
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
}

type visitor struct {
	tokens    float64
	lastSeen  time.Time
}

// Redirector is the main application handler
type Redirector struct {
	rules         []RedirectRule
	config        Config
	rateLimiter   *RateLimiter
	errorTemplate *template.Template
}

// Rate limit settings (very generous to avoid false positives)
const (
	rateLimitTokens    = 100.0 // Max tokens (requests) per IP
	rateLimitPerSecond = 10.0  // Token refill rate per second
	rateLimitCleanup   = 5 * time.Minute
)

// ErrorPageData holds data for error page template
type ErrorPageData struct {
	StatusCode int
	Title      string
	Message    string
	Host       string
}

func main() {
	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/app/config/redirects.json"
	}

	// Load error template
	templatePath := os.Getenv("TEMPLATE_PATH")
	if templatePath == "" {
		templatePath = "/app/templates/error.html"
	}

	// Create redirector instance
	redirector := &Redirector{
		config: Config{
			TrustProxy: getEnvBool("TRUST_PROXY", true),
		},
		rateLimiter:   newRateLimiter(),
		errorTemplate: loadErrorTemplate(templatePath),
	}

	if err := redirector.loadConfig(configPath); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Loaded %d redirect rules (trust_proxy=%v)", len(redirector.rules), redirector.config.TrustProxy)
	for _, rule := range redirector.rules {
		log.Printf("  %v -> %s (%d, path=%v, query=%v)",
			rule.Source, rule.Target, rule.statusCode,
			rule.PreservePath, rule.PreserveQuery)
	}

	// Get port from environment (default: 8080)
	port := 8080
	if portStr := os.Getenv("SERVER_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p <= 65535 {
			port = p
		}
	}

	// Create HTTP server with timeouts
	mux := http.NewServeMux()
	mux.HandleFunc("/health", redirector.healthHandler)
	mux.HandleFunc("/", redirector.redirectHandler)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start rate limiter cleanup goroutine
	go redirector.rateLimiter.cleanupLoop()

	log.Printf("Starting HTTP Redirector on port %d", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// getEnvBool reads a boolean environment variable with default
func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	switch strings.ToLower(val) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return defaultVal
	}
}

// loadErrorTemplate loads the error template from file, returns nil if not found
func loadErrorTemplate(path string) *template.Template {
	tmpl, err := template.ParseFiles(path)
	if err != nil {
		log.Printf("Error template not found at %s, using plain text fallback", path)
		return nil
	}
	log.Printf("Loaded error template from %s", path)
	return tmpl
}

// newRateLimiter creates a new rate limiter
func newRateLimiter() *RateLimiter {
	return &RateLimiter{
		visitors: make(map[string]*visitor),
	}
}

// allow checks if the IP is allowed to make a request
func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &visitor{tokens: rateLimitTokens - 1, lastSeen: now}
		return true
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(v.lastSeen).Seconds()
	v.tokens += elapsed * rateLimitPerSecond
	if v.tokens > rateLimitTokens {
		v.tokens = rateLimitTokens
	}
	v.lastSeen = now

	if v.tokens >= 1 {
		v.tokens--
		return true
	}
	return false
}

// cleanupLoop periodically removes old entries
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rateLimitCleanup)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, v := range rl.visitors {
			if now.Sub(v.lastSeen) > rateLimitCleanup {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rd *Redirector) loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read config file: %w", err)
	}

	if err := json.Unmarshal(data, &rd.rules); err != nil {
		return fmt.Errorf("cannot parse config: %w", err)
	}

	// Validate and parse type to status code
	for i := range rd.rules {
		rule := &rd.rules[i]

		// Parse type to status code
		statusCode, err := parseRedirectType(rule.Type)
		if err != nil {
			return fmt.Errorf("rule %d: %w", i, err)
		}
		rule.statusCode = statusCode

		// Validate source
		if len(rule.Source) == 0 {
			return fmt.Errorf("rule %d: source array cannot be empty", i)
		}

		// Validate target
		if rule.Target == "" {
			return fmt.Errorf("rule %d: target cannot be empty", i)
		}

		// Validate and parse target URL (prevent open redirect)
		parsedTarget, err := validateTargetURL(rule.Target)
		if err != nil {
			return fmt.Errorf("rule %d: %w", i, err)
		}
		rule.parsedTarget = parsedTarget
	}

	return nil
}

// validateTargetURL validates the target URL to prevent open redirect vulnerabilities
func validateTargetURL(target string) (*url.URL, error) {
	// Ensure target URL has a scheme
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "https://" + target
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL '%s': %w", target, err)
	}

	// Must have a valid host
	if parsed.Host == "" {
		return nil, fmt.Errorf("invalid target URL '%s': missing host", target)
	}

	// Prevent redirects to localhost/loopback addresses
	host := parsed.Hostname()
	if isLocalAddress(host) {
		return nil, fmt.Errorf("invalid target URL '%s': cannot redirect to local addresses", target)
	}

	// Must be http or https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("invalid target URL '%s': scheme must be http or https", target)
	}

	return parsed, nil
}

// isLocalAddress checks if the host is a local/loopback address
func isLocalAddress(host string) bool {
	// Check common local hostnames
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || lowerHost == "localhost.localdomain" {
		return true
	}

	// Check if it's an IP address
	ip := net.ParseIP(host)
	if ip != nil {
		// Check loopback (127.0.0.0/8 or ::1)
		if ip.IsLoopback() {
			return true
		}
		// Check private ranges (optional, but good for security)
		if ip.IsPrivate() {
			return true
		}
		// Check link-local
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true
		}
	}

	return false
}

// parseRedirectType converts type string to HTTP status code
func parseRedirectType(t string) (int, error) {
	if t == "" {
		return 301, nil // Default to permanent
	}

	switch strings.ToLower(t) {
	case "permanent":
		return 301, nil
	case "temporary":
		return 302, nil
	default:
		// Try to parse as numeric status code
		code, err := strconv.Atoi(t)
		if err != nil {
			return 0, fmt.Errorf("invalid type '%s': must be 'permanent', 'temporary', or a 3xx status code", t)
		}
		if code < 300 || code > 399 {
			return 0, fmt.Errorf("invalid status code %d: must be a 3xx redirect code", code)
		}
		return code, nil
	}
}

// renderErrorPage renders an error page (HTML template or plain text fallback)
func (rd *Redirector) renderErrorPage(w http.ResponseWriter, data ErrorPageData) {
	// Use HTML template if available
	if rd.errorTemplate != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(data.StatusCode)
		if err := rd.errorTemplate.Execute(w, data); err != nil {
			log.Printf("Error rendering error page: %v", err)
		}
		return
	}

	// Plain text fallback
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(data.StatusCode)
	msg := fmt.Sprintf("%d %s\n\n%s", data.StatusCode, data.Title, data.Message)
	if data.Host != "" {
		msg += fmt.Sprintf("\n\nHost: %s", data.Host)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		log.Printf("Error writing error response: %v", err)
	}
}

func (rd *Redirector) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "healthy"}); err != nil {
		log.Printf("Error encoding health response: %v", err)
	}
}

func (rd *Redirector) redirectHandler(w http.ResponseWriter, r *http.Request) {
	// Rate limiting
	clientIP := rd.getClientIP(r)
	if !rd.rateLimiter.allow(clientIP) {
		log.Printf("Rate limit exceeded for IP: %s", clientIP)
		rd.renderErrorPage(w, ErrorPageData{
			StatusCode: http.StatusTooManyRequests,
			Title:      "Too Many Requests",
			Message:    "You have sent too many requests in a short period of time. Please wait a moment and try again.",
		})
		return
	}

	// Get the original host from proxy headers (if trusted) or Host header
	host := rd.getOriginalHost(r)

	rule := rd.findMatchingRule(host)
	if rule == nil {
		log.Printf("No rule found for host: %s (X-Forwarded-Host: %s, Host: %s)",
			host, r.Header.Get("X-Forwarded-Host"), r.Host)
		rd.renderErrorPage(w, ErrorPageData{
			StatusCode: http.StatusNotFound,
			Title:      "No Redirect Configured",
			Message:    "There is no redirect rule configured for this domain.",
			Host:       host,
		})
		return
	}

	targetURL := rd.buildTargetURL(rule, r)
	log.Printf("Redirecting %s%s -> %s (%d)", host, r.URL.RequestURI(), targetURL, rule.statusCode)

	http.Redirect(w, r, targetURL, rule.statusCode)
}

// getClientIP extracts the client IP address
func (rd *Redirector) getClientIP(r *http.Request) string {
	// If we trust proxy headers, check X-Forwarded-For
	if rd.config.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For can contain multiple IPs, take the first (original client)
			if idx := strings.Index(xff, ","); idx != -1 {
				return strings.TrimSpace(xff[:idx])
			}
			return strings.TrimSpace(xff)
		}
		if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
			return strings.TrimSpace(xrip)
		}
	}

	// Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// getOriginalHost extracts the original host from proxy headers (if trusted)
// Priority: X-Forwarded-Host > Host header
func (rd *Redirector) getOriginalHost(r *http.Request) string {
	// Only trust proxy headers if configured
	if rd.config.TrustProxy {
		if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
			// X-Forwarded-Host can contain multiple hosts (comma-separated), take the first
			if idx := strings.Index(forwardedHost, ","); idx != -1 {
				forwardedHost = strings.TrimSpace(forwardedHost[:idx])
			}
			return normalizeHost(forwardedHost)
		}
	}

	// Fallback to Host header
	return normalizeHost(r.Host)
}

// normalizeHost removes port and converts to lowercase
func normalizeHost(host string) string {
	host = strings.ToLower(host)
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}

func (rd *Redirector) findMatchingRule(host string) *RedirectRule {
	// First, try exact match in source
	for i := range rd.rules {
		for _, src := range rd.rules[i].Source {
			srcHost := strings.ToLower(src)
			if srcHost == host {
				return &rd.rules[i]
			}
		}
	}

	// Then, try wildcard match (*.domain.de matches sub.domain.de)
	for i := range rd.rules {
		for _, src := range rd.rules[i].Source {
			srcHost := strings.ToLower(src)
			if strings.HasPrefix(srcHost, "*.") {
				baseDomain := srcHost[2:]
				if strings.HasSuffix(host, baseDomain) && host != baseDomain {
					return &rd.rules[i]
				}
			}
		}
	}

	// Finally, try match with www prefix handling
	for i := range rd.rules {
		for _, src := range rd.rules[i].Source {
			srcHost := strings.ToLower(src)
			// If rule is for domain.de, also match www.domain.de
			if "www."+srcHost == host {
				return &rd.rules[i]
			}
			// If rule is for www.domain.de, also match domain.de
			if strings.HasPrefix(srcHost, "www.") && srcHost[4:] == host {
				return &rd.rules[i]
			}
		}
	}

	return nil
}

func (rd *Redirector) buildTargetURL(rule *RedirectRule, r *http.Request) string {
	// Clone the pre-validated target URL
	parsed := &url.URL{
		Scheme:   rule.parsedTarget.Scheme,
		Host:     rule.parsedTarget.Host,
		Path:     rule.parsedTarget.Path,
		RawQuery: rule.parsedTarget.RawQuery,
	}

	// Preserve path if configured
	if rule.PreservePath && r.URL.Path != "" && r.URL.Path != "/" {
		// Combine paths properly
		if strings.HasSuffix(parsed.Path, "/") {
			parsed.Path = parsed.Path + strings.TrimPrefix(r.URL.Path, "/")
		} else {
			parsed.Path = parsed.Path + r.URL.Path
		}
	}

	// Preserve query parameters if configured
	if rule.PreserveQuery && r.URL.RawQuery != "" {
		if parsed.RawQuery != "" {
			parsed.RawQuery = parsed.RawQuery + "&" + r.URL.RawQuery
		} else {
			parsed.RawQuery = r.URL.RawQuery
		}
	}

	return parsed.String()
}
