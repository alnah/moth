package cli

import (
	"testing"

	"github.com/alnah/moth/internal/supervisor"
)

var _ BrowserService = (*supervisor.Client)(nil)

func TestDefaultDependenciesUseSupervisorForBrowser(t *testing.T) {
	set := defaultDependencies(defaultDependencyOptions{})
	t.Cleanup(func() { _ = set.supervisorClient.Close() })

	client, ok := set.Browser.(*supervisor.Client)
	if !ok {
		t.Fatalf("Dependencies.Browser = %T, want *supervisor.Client", set.Browser)
	}
	if client != set.supervisorClient {
		t.Fatal("Dependencies.Browser and supervisorClient are different instances")
	}
}
