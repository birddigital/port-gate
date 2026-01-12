package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	// Default port for the gate itself
	DefaultPort = 80
	// System index config directory
	SystemIndexDir = "~/.system-index"
)

// Service represents a service from system-index
type Service struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Port int    `json:"port,omitempty"`
	Host string `json:"host,omitempty"`
}

// Config is the port-gate configuration
type Config struct {
	Port         int                 `json:"port"`
	DomainSuffix string              `json:"domain_suffix"`
	Services     map[string]int      `json:"services"`  // domain -> port
	Headers      map[string]string   `json:"headers"`
}

// PortGate is the main reverse proxy
type PortGate struct {
	config      *Config
	configPath  string
	http        *http.Server
	mu          sync.RWMutex
	hostsFile   string
}

func main() {
	configPath := getConfigPath()
	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Printf("[WARN] No config found, creating default: %v", err)
		cfg = &Config{
			Port:         DefaultPort,
			DomainSuffix: ".local",
			Services:     make(map[string]int),
			Headers: map[string]string{
				"X-Forwarded-Proto": "http",
				"X-Real-IP":         "127.0.0.1",
			},
		}
		// Auto-import from system-index
		importFromSystemIndex(cfg)
		saveConfig(configPath, cfg)
	}

	gate, err := NewPortGate(cfg, configPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to create gate: %v", err)
	}

	// Watch for config changes
	go gate.watchConfig()

	// Print hosts entries
	gate.printHostsInstructions()

	// Start server
	if err := gate.Start(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[FATAL] Server error: %v", err)
	}
}

// NewPortGate creates a new PortGate
func NewPortGate(cfg *Config, configPath string) (*PortGate, error) {
	// Default to localhost if binding to port 80 fails without root
	if cfg.Port == 80 && os.Geteuid() != 0 {
		log.Println("[WARN] Port 80 requires root. Using 8080 instead.")
		cfg.Port = 8080
	}

	mux := http.NewServeMux()
	gate := &PortGate{
		config:     cfg,
		configPath: configPath,
		hostsFile:  "/etc/hosts",
	}

	// Register catch-all handler
	mux.HandleFunc("/", gate.handleRequest)

	gate.http = &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	return gate, nil
}

// Start begins serving HTTP requests
func (g *PortGate) Start() error {
	log.Printf("[START] Port-Gate listening on :%d", g.config.Port)
	log.Printf("[INFO] Domain suffix: %s", g.config.DomainSuffix)
	log.Printf("[INFO] Services: %d", len(g.config.Services))

	return g.http.ListenAndServe()
}

// handleRequest routes requests to the appropriate backend
func (g *PortGate) handleRequest(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Check if this is a .local domain we should proxy
	var backendPort int
	var ok bool

	g.mu.RLock()
	for domain, port := range g.config.Services {
		if host == domain || host == domain+g.config.DomainSuffix {
			backendPort = port
			ok = true
			break
		}
	}
	g.mu.RUnlock()

	if !ok {
		// Try to find by prefix (e.g., "taskflow" matches "taskflow.local")
		g.mu.RLock()
		for domain, port := range g.config.Services {
			if strings.HasPrefix(host, domain) || strings.HasPrefix(domain, host) {
				backendPort = port
				ok = true
				break
			}
		}
		g.mu.RUnlock()
	}

	if !ok {
		http.Error(w, fmt.Sprintf("No service found for host: %s\nAvailable: %v", host, g.getAvailableDomains()), http.StatusBadGateway)
		return
	}

	// Build backend URL
	backendURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", backendPort),
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	// Set custom headers
	if g.config.Headers != nil {
		oldDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			oldDirector(req)
			for k, v := range g.config.Headers {
				req.Header.Set(k, v)
			}
		}
	}

	// Log the proxy request
	log.Printf("[PROXY] %s -> http://localhost:%d%s", r.Host, backendPort, r.URL.Path)

	proxy.ServeHTTP(w, r)
}

// getAvailableDomains returns list of available domains
func (g *PortGate) getAvailableDomains() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	domains := make([]string, 0, len(g.config.Services))
	for d := range g.config.Services {
		domains = append(domains, d+g.config.DomainSuffix)
	}
	return domains
}

// watchConfig monitors config file for changes
func (g *PortGate) watchConfig() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[WARN] Cannot watch config: %v", err)
		return
	}
	defer watcher.Close()

	configDir := filepath.Dir(g.configPath)
	watcher.Add(configDir)

	// Also watch system-index services dir
	systemIndexDir := expandPath(SystemIndexDir)
	if _, err := os.Stat(systemIndexDir); err == nil {
		watcher.Add(systemIndexDir)
		watcher.Add(filepath.Join(systemIndexDir, "services"))
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				// Reload config
				if newCfg, err := loadConfig(g.configPath); err == nil {
					g.mu.Lock()
					g.config = newCfg
					g.mu.Unlock()
					log.Printf("[RELOAD] Configuration reloaded")
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[ERROR] Watcher error: %v", err)
		}
	}
}

// printHostsInstructions shows what to add to /etc/hosts
func (g *PortGate) printHostsInstructions() {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("  PORT-GATE - Add these entries to /etc/hosts:")
	fmt.Println(strings.Repeat("=", 60))

	g.mu.RLock()
	defer g.mu.RUnlock()

	for domain := range g.config.Services {
		fullDomain := domain + g.config.DomainSuffix
		fmt.Printf("  127.0.0.1  %s\n", fullDomain)
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("  Then access services at:\n")
	for domain := range g.config.Services {
		fullDomain := domain + g.config.DomainSuffix
		fmt.Printf("    http://%s/\n", fullDomain)
	}
	fmt.Println(strings.Repeat("=", 60) + "\n")
}

// Stop gracefully shuts down the server
func (g *PortGate) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return g.http.Shutdown(ctx)
}

// getConfigPath returns the config file path
func getConfigPath() string {
	// Check for config in current directory first
	if _, err := os.Stat("port-gate.json"); err == nil {
		return "port-gate.json"
	}
	// Then check ~/.system-index/
	dir := expandPath(SystemIndexDir)
	return filepath.Join(dir, "port-gate.json")
}

// loadConfig loads configuration from file
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Auto-import from system-index if services is empty
	if len(cfg.Services) == 0 {
		importFromSystemIndex(&cfg)
	}

	return &cfg, nil
}

// saveConfig writes configuration to file
func saveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// importFromSystemIndex reads services from system-index configs
func importFromSystemIndex(cfg *Config) {
	systemIndexDir := expandPath(SystemIndexDir)
	servicesDir := filepath.Join(systemIndexDir, "services")

	entries, err := os.ReadDir(servicesDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(servicesDir, entry.Name()))
		if err != nil {
			continue
		}

		var svc Service
		if err := json.Unmarshal(data, &svc); err != nil {
			continue
		}

		if svc.Port > 0 {
			// Generate domain from service ID or name
			domain := strings.ToLower(svc.ID)
			if domain == "" {
				domain = strings.ToLower(svc.Name)
				domain = strings.ReplaceAll(domain, " ", "-")
			}
			// Remove "-web" suffix for cleaner URLs
			domain = strings.TrimSuffix(domain, "-web")

			cfg.Services[domain] = svc.Port
			log.Printf("[IMPORT] %s -> port %d", domain, svc.Port)
		}
	}
}

// expandPath expands ~ to home directory
func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func init() {
	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("[STOP] Shutting down...")
		os.Exit(0)
	}()
}
