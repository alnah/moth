package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alnah/moth/internal/browser"
)

type browserSessionFlags struct {
	Profile string
	Session string
	PageID  string
}

type browserScopeFlags struct {
	Scope string
}

func addBrowserCommand(root *cobra.Command, rootOptions *rootFlags, deps Dependencies) {
	browserCmd := &cobra.Command{Use: "browser", Short: "Run browser operations"}
	browserCmd.AddCommand(
		newBrowserStartCommand(rootOptions, deps),
		newBrowserStopCommand(rootOptions, deps),
		newBrowserStatusCommand(rootOptions, deps),
		newBrowserConnectCommand(rootOptions, deps),
		newBrowserOpenCommand(rootOptions, deps),
		newBrowserPagesCommand(rootOptions, deps),
		newBrowserPageCommand(rootOptions, deps),
		newBrowserClosePageCommand(rootOptions, deps),
		newBrowserClickCommand(rootOptions, deps),
		newBrowserInputCommand(rootOptions, deps),
		newBrowserWaitCommand(rootOptions, deps),
		newBrowserScreenshotCommand(rootOptions, deps),
		newBrowserPDFCommand(rootOptions, deps),
		newBrowserDownloadCommand(rootOptions, deps),
		newBrowserMetadataCommand(rootOptions, deps),
		newBrowserAXTreeCommand(rootOptions, deps),
		newBrowserChallengeCommand(rootOptions, deps),
	)
	root.AddCommand(browserCmd)
}

func newBrowserStartCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserScopeFlags{Scope: "auto"}
	show := false
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a persistent browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return newInvalidArgumentsError(errors.New("browser start accepts no positional arguments"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			status, err := deps.Browser.Start(ctx, browser.StartRequest{Scope: flags.Scope, Show: show})
			if err != nil {
				return fmt.Errorf("browser start: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserStatusResult(status))
		},
	}
	cmd.Flags().BoolVar(&show, "show", false, "show browser window")
	addBrowserScopeFlags(cmd, &flags)
	return cmd
}

func newBrowserStopCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserScopeFlags{Scope: "auto"}
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a persistent browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return newInvalidArgumentsError(errors.New("browser stop accepts no positional arguments"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			status, err := deps.Browser.Stop(ctx, browser.StopRequest{Scope: flags.Scope})
			if err != nil {
				return fmt.Errorf("browser stop: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserStatusResult(status))
		},
	}
	addBrowserScopeFlags(cmd, &flags)
	return cmd
}

func newBrowserStatusCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserScopeFlags{Scope: "auto"}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show persistent browser status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return newInvalidArgumentsError(errors.New("browser status accepts no positional arguments"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			status, err := deps.Browser.Status(ctx, browser.StatusRequest{Scope: flags.Scope})
			if err != nil {
				return fmt.Errorf("browser status: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserStatusResult(status))
		},
	}
	addBrowserScopeFlags(cmd, &flags)
	return cmd
}

func newBrowserConnectCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserScopeFlags{Scope: "auto"}
	cmd := &cobra.Command{
		Use:   "connect <host:port>",
		Short: "Connect to an external browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("browser connect accepts exactly one host:port"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			status, err := deps.Browser.Connect(ctx, browser.ConnectRequest{Scope: flags.Scope, HostPort: args[0]})
			if err != nil {
				return fmt.Errorf("browser connect: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserStatusResult(status))
		},
	}
	addBrowserScopeFlags(cmd, &flags)
	return cmd
}

func newBrowserOpenCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserSessionFlags{}
	cmd := &cobra.Command{
		Use:   "open <url>",
		Short: "Open a persistent browser page",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("browser open accepts exactly one URL"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			page, err := deps.Browser.OpenPage(ctx, browser.OpenPageRequest{
				URL:         args[0],
				ProfileName: flags.Profile,
				SessionName: flags.Session,
			})
			if err != nil {
				return fmt.Errorf("browser open: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserPageResult(page))
		},
	}
	addBrowserSessionFlags(cmd, &flags)
	return cmd
}

func newBrowserPagesCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserSessionFlags{}
	cmd := &cobra.Command{
		Use:   "pages",
		Short: "List browser pages",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return newInvalidArgumentsError(errors.New("browser pages accepts no positional arguments"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			pages, err := deps.Browser.ListPages(ctx, browser.SessionRequest{
				ProfileName: flags.Profile,
				SessionName: flags.Session,
			})
			if err != nil {
				return fmt.Errorf("browser pages: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserPagesResult(pages))
		},
	}
	addBrowserSessionFlags(cmd, &flags)
	return cmd
}

func newBrowserPageCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserSessionFlags{}
	cmd := &cobra.Command{
		Use:   "page <page-id>",
		Short: "Select a browser page",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("browser page accepts exactly one page ID"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			page, err := deps.Browser.SwitchPage(ctx, pageSelection(flags, args[0]))
			if err != nil {
				return fmt.Errorf("browser page: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserPageResult(page))
		},
	}
	addBrowserSessionFlags(cmd, &flags)
	return cmd
}

func newBrowserClosePageCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserSessionFlags{}
	cmd := &cobra.Command{
		Use:   "close-page [page-id]",
		Short: "Close a browser page",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return newInvalidArgumentsError(errors.New("browser close-page accepts at most one page ID"))
			}
			pageID := flags.PageID
			if len(args) == 1 {
				pageID = args[0]
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			if err := deps.Browser.ClosePage(ctx, pageSelection(flags, pageID)); err != nil {
				return fmt.Errorf("browser close-page: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserOperationResult())
		},
	}
	addBrowserPageFlags(cmd, &flags)
	return cmd
}

func newBrowserClickCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserSessionFlags{}
	cmd := &cobra.Command{
		Use:   "click <selector>",
		Short: "Click a browser element",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("browser click accepts exactly one selector"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			if err := deps.Browser.Click(ctx, browser.InteractionRequest{
				ProfileName: flags.Profile,
				SessionName: flags.Session,
				PageID:      flags.PageID,
				Selector:    args[0],
			}); err != nil {
				return fmt.Errorf("browser click: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserOperationResult())
		},
	}
	addBrowserPageFlags(cmd, &flags)
	return cmd
}

func newBrowserInputCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserSessionFlags{}
	cmd := &cobra.Command{
		Use:   "input <selector> <text>",
		Short: "Type text into a browser element",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return newInvalidArgumentsError(errors.New("browser input accepts selector and text"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			if err := deps.Browser.Input(ctx, browser.InputRequest{
				ProfileName: flags.Profile,
				SessionName: flags.Session,
				PageID:      flags.PageID,
				Selector:    args[0],
				Text:        args[1],
			}); err != nil {
				return fmt.Errorf("browser input: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserOperationResult())
		},
	}
	addBrowserPageFlags(cmd, &flags)
	return cmd
}

func newBrowserWaitCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserSessionFlags{}
	state := string(browser.WaitAttached)
	cmd := &cobra.Command{
		Use:   "wait <selector>",
		Short: "Wait for a browser selector",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("browser wait accepts exactly one selector"))
			}
			waitState := browser.WaitState(state)
			if waitState != browser.WaitAttached && waitState != browser.WaitVisible {
				return newInvalidArgumentsError(fmt.Errorf("invalid wait state %q", state))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			if err := deps.Browser.Wait(ctx, browser.WaitRequest{
				ProfileName: flags.Profile,
				SessionName: flags.Session,
				PageID:      flags.PageID,
				Selector:    args[0],
				State:       waitState,
			}); err != nil {
				return fmt.Errorf("browser wait: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserOperationResult())
		},
	}
	cmd.Flags().StringVar(&state, "state", state, "wait state")
	addBrowserPageFlags(cmd, &flags)
	return cmd
}

func newBrowserMetadataCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	maxHeaderBytes := 0
	cmd := &cobra.Command{
		Use:   "metadata <url>",
		Short: "Capture response metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("browser metadata accepts exactly one URL"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			metadata, err := deps.Browser.ResponseMetadata(ctx, browser.ResponseMetadataRequest{
				URL:            args[0],
				MaxHeaderBytes: maxHeaderBytes,
			})
			if err != nil {
				return fmt.Errorf("browser metadata: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserMetadataResult(metadata))
		},
	}
	cmd.Flags().IntVar(&maxHeaderBytes, "max-header-bytes", 0, "maximum header bytes")
	return cmd
}

func newBrowserAXTreeCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserSessionFlags{}
	maxDepth := 0
	cmd := &cobra.Command{
		Use:   "ax-tree",
		Short: "Extract accessibility tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return newInvalidArgumentsError(errors.New("browser ax-tree accepts no positional arguments"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			tree, err := deps.Browser.AccessibilityTree(ctx, browser.AccessibilityRequest{
				ProfileName: flags.Profile,
				SessionName: flags.Session,
				PageID:      flags.PageID,
				MaxDepth:    maxDepth,
			})
			if err != nil {
				return fmt.Errorf("browser ax-tree: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserAccessibilityResult(tree))
		},
	}
	cmd.Flags().IntVar(&maxDepth, "max-depth", 0, "maximum tree depth")
	addBrowserPageFlags(cmd, &flags)
	return cmd
}

func newBrowserChallengeCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserSessionFlags{}
	cmd := &cobra.Command{
		Use:   "challenge",
		Short: "Detect manual challenge state",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return newInvalidArgumentsError(errors.New("browser challenge accepts no positional arguments"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			challenge, err := deps.Browser.DetectManualChallenge(ctx, browser.ManualChallengeRequest{
				ProfileName: flags.Profile,
				SessionName: flags.Session,
				PageID:      flags.PageID,
			})
			if err != nil {
				return fmt.Errorf("browser challenge: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserChallengeResult(challenge))
		},
	}
	addBrowserPageFlags(cmd, &flags)
	return cmd
}

func newBrowserScreenshotCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	fullPage := false
	cmd := &cobra.Command{
		Use:   "screenshot <url> <path>",
		Short: "Capture a page screenshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return newInvalidArgumentsError(errors.New("browser screenshot accepts URL and path"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			request := browser.ScreenshotRequest{
				URL:      args[0],
				Path:     args[1],
				FullPage: fullPage,
				MaxBytes: rootOptions.Limits.MaxBytes,
			}
			if err := deps.Browser.Screenshot(ctx, request); err != nil {
				return fmt.Errorf("browser screenshot: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, screenshotPack(request))
		},
	}
	cmd.Flags().BoolVar(&fullPage, "full-page", false, "capture full page")
	return cmd
}

func newBrowserPDFCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pdf <url> <path>",
		Short: "Capture a page PDF",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return newInvalidArgumentsError(errors.New("browser pdf accepts URL and path"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			request := browser.PDFRequest{URL: args[0], Path: args[1], MaxBytes: rootOptions.Limits.MaxBytes}
			if err := deps.Browser.PDF(ctx, request); err != nil {
				return fmt.Errorf("browser pdf: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, browserPDFPack(request))
		},
	}
	return cmd
}

func newBrowserDownloadCommand(rootOptions *rootFlags, deps Dependencies) *cobra.Command {
	flags := browserSessionFlags{}
	cmd := &cobra.Command{
		Use:   "download <selector> <path>",
		Short: "Capture a browser download",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return newInvalidArgumentsError(errors.New("browser download accepts selector and path"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			result, err := deps.Browser.Download(ctx, browser.DownloadRequest{
				ProfileName: flags.Profile,
				SessionName: flags.Session,
				PageID:      flags.PageID,
				Selector:    args[0],
				Path:        args[1],
			})
			if err != nil {
				return fmt.Errorf("browser download: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, downloadPack(result))
		},
	}
	addBrowserPageFlags(cmd, &flags)
	return cmd
}

func addBrowserScopeFlags(cmd *cobra.Command, flags *browserScopeFlags) {
	cmd.Flags().BoolFunc("local", "use local browser state", func(_ string) error {
		flags.Scope = "local"
		return nil
	})
	cmd.Flags().BoolFunc("global", "use global browser state", func(_ string) error {
		flags.Scope = "global"
		return nil
	})
}

func addBrowserSessionFlags(cmd *cobra.Command, flags *browserSessionFlags) {
	cmd.Flags().StringVar(&flags.Profile, "profile", "", "browser profile")
	cmd.Flags().StringVar(&flags.Session, "session", "", "browser session")
}

func addBrowserPageFlags(cmd *cobra.Command, flags *browserSessionFlags) {
	addBrowserSessionFlags(cmd, flags)
	cmd.Flags().StringVar(&flags.PageID, "page", "", "browser page ID")
}

func pageSelection(flags browserSessionFlags, pageID string) browser.PageSelection {
	return browser.PageSelection{ProfileName: flags.Profile, SessionName: flags.Session, PageID: pageID}
}
