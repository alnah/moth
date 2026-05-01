package browser

import (
	"path/filepath"
	"strings"
)

// Option changes browser pool and worker creation behavior.
type Option func(*poolConfig)

type poolConfig struct {
	workerFactory           WorkerFactory
	configuredWorkerFactory ConfiguredWorkerFactory
	browserBin              string
	noSandbox               bool
	profileRoot             string
	profileName             string
	headless                bool
	proxyURL                string
	blockedResources        ResourceSet
}

func defaultPoolConfig() poolConfig {
	return poolConfig{headless: true}
}

// WithWorkerFactory replaces the Rod worker factory, mainly for deterministic tests.
func WithWorkerFactory(factory WorkerFactory) Option {
	return func(config *poolConfig) {
		config.workerFactory = factory
	}
}

// WithConfiguredWorkerFactory replaces the Rod worker factory and receives exported options.
func WithConfiguredWorkerFactory(factory ConfiguredWorkerFactory) Option {
	return func(config *poolConfig) {
		config.configuredWorkerFactory = factory
	}
}

// WithBrowserBin uses an explicit Chromium-compatible browser binary.
func WithBrowserBin(path string) Option {
	return func(config *poolConfig) {
		config.browserBin = path
	}
}

// WithNoSandbox controls Chromium sandbox usage for CI or container environments.
func WithNoSandbox(noSandbox bool) Option {
	return func(config *poolConfig) {
		config.noSandbox = noSandbox
	}
}

// WithProfileRoot stores named browser profiles under root.
func WithProfileRoot(root string) Option {
	return func(config *poolConfig) {
		config.profileRoot = root
	}
}

// WithProfileName uses an isolated persistent user-data directory for name.
func WithProfileName(name string) Option {
	return func(config *poolConfig) {
		config.profileName = name
	}
}

// WithProxy configures a normal browser proxy server.
func WithProxy(proxyURL string) Option {
	return func(config *poolConfig) {
		config.proxyURL = proxyURL
	}
}

// WithBlockedResources blocks browser resource classes.
func WithBlockedResources(resources ...Resource) Option {
	return func(config *poolConfig) {
		for _, resource := range resources {
			config.blockedResources |= ResourceSet(resource)
		}
	}
}

func (config poolConfig) workerOptions() WorkerOptions {
	return WorkerOptions{
		BrowserBin:       config.browserBin,
		NoSandbox:        config.noSandbox,
		UserDataDir:      config.userDataDir(),
		Headless:         config.headless,
		ProxyURL:         config.proxyURL,
		BlockedResources: config.blockedResources,
	}
}

func (config poolConfig) userDataDir() string {
	if config.profileRoot == "" || config.profileName == "" {
		return ""
	}
	return filepath.Join(config.profileRoot, "profiles", safeProfileName(config.profileName))
}

func safeProfileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "default"
	}
	return strings.Map(func(char rune) rune {
		switch char {
		case '/', '\\', ':':
			return '_'
		default:
			return char
		}
	}, name)
}
