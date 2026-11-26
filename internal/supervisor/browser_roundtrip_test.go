//go:build !windows

//nolint:gocyclo,govet // Round-trip tests keep protocol sequences explicit.
package supervisor_test

import (
	"context"
	"testing"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/supervisor"
)

func TestServerBrowserPersistentCommandsRoundTrip(t *testing.T) {
	cwd, dirs := browserTestStateDirs(t)
	launcher := &browserTestLauncher{result: browser.LaunchResult{DebugURL: "ws://owned", ChromePID: 4242}}
	connection := newBrowserTestConnection()
	connector := newBrowserTestConnector(map[string]*browserTestConnection{"ws://owned": connection})
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{
		PersistentServiceFactory: newBrowserTestFactory(launcher, connector, nil),
	})
	headers := browserRequestMetadata(t, cwd, dirs.Local, dirs.Global)

	decodeBrowserResult[supervisor.BrowserStatusResult](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserStart, supervisor.BrowserStartParams{Scope: "local"}, headers,
	))
	page := decodeBrowserResult[supervisor.BrowserPageResult](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserOpen,
		supervisor.BrowserOpenParams{URL: "https://example.test/page"}, headers,
	))
	status := decodeBrowserResult[supervisor.BrowserStatusResult](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserStatus, supervisor.BrowserStatusParams{}, headers,
	))
	if status.Status != "running" {
		t.Fatalf("browser.status = %#v, want running", status)
	}
	selected := decodeBrowserResult[supervisor.BrowserPageResult](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserSwitch,
		supervisor.BrowserSelectionParams{PageID: page.ID}, headers,
	))
	if selected.ID != page.ID {
		t.Fatalf("browser.switch = %#v, want page %q", selected, page.ID)
	}
	decodeBrowserResult[struct{}](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserClick,
		supervisor.BrowserInteractionParams{Selector: "button"}, headers,
	))
	decodeBrowserResult[struct{}](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserInput,
		supervisor.BrowserInputParams{Selector: "input", Text: "moth"}, headers,
	))
	decodeBrowserResult[struct{}](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserWait,
		supervisor.BrowserWaitParams{Selector: "main", State: "visible"}, headers,
	))
	tree := decodeBrowserResult[supervisor.BrowserAccessibilityResult](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserAccessibility,
		supervisor.BrowserAccessibilityParams{}, headers,
	))
	if len(tree.Nodes) == 0 {
		t.Fatal("browser.accessibility returned no nodes")
	}
	challenge := decodeBrowserResult[supervisor.BrowserChallengeResult](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserChallenge,
		supervisor.BrowserChallengeParams{}, headers,
	))
	if !challenge.ManualRequired || len(challenge.Warnings) == 0 {
		t.Fatalf("browser.challenge = %#v, want manual challenge warning", challenge)
	}
	decodeBrowserResult[struct{}](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserClosePage,
		supervisor.BrowserSelectionParams{PageID: page.ID}, headers,
	))
	stopped := decodeBrowserResult[supervisor.BrowserStatusResult](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserStop, supervisor.BrowserStopParams{Scope: "local"}, headers,
	))
	if stopped.Status != "stopped" {
		t.Fatalf("browser.stop = %#v, want stopped", stopped)
	}
}

func TestClientBrowserPersistentCommandsRoundTrip(t *testing.T) {
	client, _ := newBrowserClient(t, func(call browserClientCall) supervisor.Reply {
		switch call.method {
		case supervisor.MethodBrowserStop, supervisor.MethodBrowserStatus, supervisor.MethodBrowserConnect:
			return browserClientReply(t, supervisor.BrowserStatusResult{
				Status: "running",
				Scope:  "local",
				Owned:  true,
			})
		case supervisor.MethodBrowserSwitch:
			return browserClientReply(t, supervisor.BrowserPageResult{ID: "page-1", Active: true})
		case supervisor.MethodBrowserAccessibility:
			return browserClientReply(t, supervisor.BrowserAccessibilityResult{
				Nodes: []supervisor.BrowserAccessibilityNode{{Role: "button", Name: "Submit"}},
			})
		case supervisor.MethodBrowserChallenge:
			return browserClientReply(t, supervisor.BrowserChallengeResult{
				ManualRequired: true,
				Kind:           "captcha",
				Warnings:       []string{string(content.WarningCaptchaPossible)},
			})
		default:
			return browserClientReply(t, struct{}{})
		}
	})

	status, err := client.Stop(context.Background(), browser.StopRequest{Scope: "local"})
	if err != nil || status.Status != "running" {
		t.Fatalf("Stop() = %#v, %v", status, err)
	}
	status, err = client.Status(context.Background(), browser.StatusRequest{Scope: "local"})
	if err != nil || status.Scope != "local" {
		t.Fatalf("Status() = %#v, %v", status, err)
	}
	status, err = client.Connect(context.Background(), browser.ConnectRequest{HostPort: "127.0.0.1:9222"})
	if err != nil || !status.Owned {
		t.Fatalf("Connect() = %#v, %v", status, err)
	}
	page, err := client.SwitchPage(context.Background(), browser.PageSelection{PageID: "page-1"})
	if err != nil || page.ID != "page-1" {
		t.Fatalf("SwitchPage() = %#v, %v", page, err)
	}
	if err := client.ClosePage(context.Background(), browser.PageSelection{PageID: "page-1"}); err != nil {
		t.Fatalf("ClosePage() error = %v", err)
	}
	if err := client.Click(context.Background(), browser.InteractionRequest{Selector: "button"}); err != nil {
		t.Fatalf("Click() error = %v", err)
	}
	if err := client.Input(context.Background(), browser.InputRequest{Selector: "input", Text: "moth"}); err != nil {
		t.Fatalf("Input() error = %v", err)
	}
	if err := client.Wait(context.Background(), browser.WaitRequest{
		Selector: "main",
		State:    browser.WaitVisible,
	}); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	tree, err := client.AccessibilityTree(context.Background(), browser.AccessibilityRequest{})
	if err != nil || len(tree.Nodes) != 1 {
		t.Fatalf("AccessibilityTree() = %#v, %v", tree, err)
	}
	challenge, err := client.DetectManualChallenge(context.Background(), browser.ManualChallengeRequest{})
	if err != nil || !challenge.ManualRequired || len(challenge.Warnings) != 1 {
		t.Fatalf("DetectManualChallenge() = %#v, %v", challenge, err)
	}
}
