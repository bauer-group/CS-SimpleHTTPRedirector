package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// RedirectRule defines a single redirect configuration
type RedirectRule struct {
	Source        []string `json:"source"`         // e.g., ["alte-domain.de", "www.alte-domain.de"]
	Target        string   `json:"target"`         // e.g., "https://neue-domain.de"
	Type          string   `json:"type"`           // "permanent", "temporary", or HTTP status code (301, 302, 307, 308)
	PreservePath  bool     `json:"preserve_path"`  // Keep the original path
	PreserveQuery bool     `json:"preserve_query"` // Keep query parameters
	statusCode    int      // Parsed status code (internal)
}

var rules []RedirectRule

func main() {
	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/app/config/redirects.json"
	}

	if err := loadConfig(configPath); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Loaded %d redirect rules", len(rules))
	for _, rule := range rules {
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

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/", redirectHandler)

	log.Printf("Starting HTTP Redirector on port %d", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read config file: %w", err)
	}

	if err := json.Unmarshal(data, &rules); err != nil {
		return fmt.Errorf("cannot parse config: %w", err)
	}

	// Validate and parse type to status code
	for i := range rules {
		rule := &rules[i]

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
	}

	return nil
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

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func redirectHandler(w http.ResponseWriter, r *http.Request) {
	// Get the original host from proxy headers (Traefik sets these)
	host := getOriginalHost(r)

	rule := findMatchingRule(host)
	if rule == nil {
		log.Printf("No rule found for host: %s (X-Forwarded-Host: %s, Host: %s)",
			host, r.Header.Get("X-Forwarded-Host"), r.Host)
		http.Error(w, "No redirect configured for this host", http.StatusNotFound)
		return
	}

	targetURL := buildTargetURL(rule, r)
	log.Printf("Redirecting %s%s -> %s (%d)", host, r.URL.RequestURI(), targetURL, rule.statusCode)

	http.Redirect(w, r, targetURL, rule.statusCode)
}

// getOriginalHost extracts the original host from proxy headers
// Priority: X-Forwarded-Host > Host header
func getOriginalHost(r *http.Request) string {
	// Check X-Forwarded-Host first (set by Traefik/reverse proxies)
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		// X-Forwarded-Host can contain multiple hosts (comma-separated), take the first
		if idx := strings.Index(forwardedHost, ","); idx != -1 {
			forwardedHost = strings.TrimSpace(forwardedHost[:idx])
		}
		return normalizeHost(forwardedHost)
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

func findMatchingRule(host string) *RedirectRule {
	// First, try exact match in source
	for i := range rules {
		for _, src := range rules[i].Source {
			srcHost := strings.ToLower(src)
			if srcHost == host {
				return &rules[i]
			}
		}
	}

	// Then, try wildcard match (*.domain.de matches sub.domain.de)
	for i := range rules {
		for _, src := range rules[i].Source {
			srcHost := strings.ToLower(src)
			if strings.HasPrefix(srcHost, "*.") {
				baseDomain := srcHost[2:]
				if strings.HasSuffix(host, baseDomain) && host != baseDomain {
					return &rules[i]
				}
			}
		}
	}

	// Finally, try match with www prefix handling
	for i := range rules {
		for _, src := range rules[i].Source {
			srcHost := strings.ToLower(src)
			// If rule is for domain.de, also match www.domain.de
			if "www."+srcHost == host {
				return &rules[i]
			}
			// If rule is for www.domain.de, also match domain.de
			if strings.HasPrefix(srcHost, "www.") && srcHost[4:] == host {
				return &rules[i]
			}
		}
	}

	return nil
}

func buildTargetURL(rule *RedirectRule, r *http.Request) string {
	targetURL := rule.Target

	// Ensure target URL has a scheme
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	// Parse the target URL
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return targetURL
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
