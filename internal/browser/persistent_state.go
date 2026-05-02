package browser

import (
	"os"
	"path/filepath"
	"strings"
)

func defaultStateDirs() StateDirs {
	local := filepath.Join(".moth", "browser")
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.TempDir()
	}
	return StateDirs{Local: local, Global: filepath.Join(home, ".moth", "browser")}
}

func normalizedScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case "global":
		return "global"
	case "local":
		return "local"
	default:
		return "auto"
	}
}

func stateFileExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, persistentStateFileName))
	return err == nil
}

func selectedPageID(explicit string, active string) string {
	if explicit != "" {
		return explicit
	}
	return active
}

func activePageID(pages []PageInfo) string {
	for _, page := range pages {
		if page.Active {
			return page.ID
		}
	}
	if len(pages) == 0 {
		return ""
	}
	return pages[0].ID
}
