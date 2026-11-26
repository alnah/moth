package supervisor

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alnah/moth/internal/browser"
)

const (
	headerCWD            = "X-Moth-CWD"
	headerLocalStateDir  = "X-Moth-Local-State-Dir"
	headerGlobalStateDir = "X-Moth-Global-State-Dir"
)

type requestMetadata struct {
	CWD            string
	LocalStateDir  string
	GlobalStateDir string
}

type browserServiceKey struct {
	Local  string
	Global string
}

func requestMetadataFrom(request *http.Request) requestMetadata {
	return requestMetadata{
		CWD:            request.Header.Get(headerCWD),
		LocalStateDir:  request.Header.Get(headerLocalStateDir),
		GlobalStateDir: request.Header.Get(headerGlobalStateDir),
	}
}

func absoluteHeaderPath(name string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: %s is required", errInvalidParams, name)
	}
	if strings.IndexByte(value, 0) >= 0 || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("%w: %s is invalid", errInvalidParams, name)
	}
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("%w: %s must be absolute", errInvalidParams, name)
	}
	return filepath.Clean(value), nil
}

func resolveStateDirs(metadata requestMetadata) (browser.StateDirs, error) {
	if _, err := absoluteHeaderPath(headerCWD, metadata.CWD); err != nil {
		return browser.StateDirs{}, err
	}
	local, err := absoluteHeaderPath(headerLocalStateDir, metadata.LocalStateDir)
	if err != nil {
		return browser.StateDirs{}, err
	}
	global, err := absoluteHeaderPath(headerGlobalStateDir, metadata.GlobalStateDir)
	if err != nil {
		return browser.StateDirs{}, err
	}
	return browser.StateDirs{Local: local, Global: global}, nil
}

func resolveClientPath(cwd string, path string) (string, error) {
	cwd, err := absoluteHeaderPath(headerCWD, cwd)
	if err != nil {
		return "", err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%w: path is required", errInvalidParams)
	}
	if strings.IndexByte(path, 0) >= 0 || strings.ContainsAny(path, "\r\n") {
		return "", fmt.Errorf("%w: path is invalid", errInvalidParams)
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Join(cwd, path), nil
}

func (server *Server) useBrowser(
	metadata requestMetadata,
	run func(*browser.PersistentService) error,
) error {
	dirs, err := resolveStateDirs(metadata)
	if err != nil {
		return err
	}
	key := browserServiceKey{Local: dirs.Local, Global: dirs.Global}

	server.browserMu.Lock()
	defer server.browserMu.Unlock()
	if server.closed.Load() {
		return errors.New("daemon closing")
	}
	service := server.browsers[key]
	if service == nil {
		service = server.newBrowserService(browser.PersistentServiceOptions{
			StateDirs:  dirs,
			BrowserBin: server.browserBin,
			Lifecycle:  server.lifecycle,
			Clock:      server.clock,
		})
		if service == nil {
			return errors.New("browser service unavailable")
		}
		server.browsers[key] = service
	}
	return run(service)
}

func sortedBrowserServiceKeys(services map[browserServiceKey]*browser.PersistentService) []browserServiceKey {
	keys := make([]browserServiceKey, 0, len(services))
	for key := range services {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i int, j int) bool {
		if keys[i].Local != keys[j].Local {
			return keys[i].Local < keys[j].Local
		}
		return keys[i].Global < keys[j].Global
	})
	return keys
}
