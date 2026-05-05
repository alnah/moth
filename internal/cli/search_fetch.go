package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alnah/moth/internal/webfetch"
	"github.com/alnah/moth/internal/websearch"
)

type searchFlags struct {
	Count      int
	Country    string
	Language   string
	SafeSearch string
	Offset     int
}

type fetchFlags struct {
	UseBrowser  bool
	IncludeText bool
}

func addSearchCommand(root *cobra.Command, rootOptions *rootFlags, deps *Dependencies) {
	searchCmd := &cobra.Command{Use: "search", Short: "Search web content"}
	searchCmd.AddCommand(newSearchKindCommand(
		"web",
		rootOptions,
		deps,
		func(ctx commandCallContext, options websearch.Options) error {
			pack, err := deps.WebSearch.SearchWeb(ctx.Context, options)
			if err != nil {
				return fmt.Errorf("search web: %w", err)
			}
			return renderResult(ctx.Command, rootOptions.Output, pack)
		},
	))
	searchCmd.AddCommand(newSearchKindCommand(
		"images",
		rootOptions,
		deps,
		func(ctx commandCallContext, options websearch.Options) error {
			pack, err := deps.WebSearch.SearchImages(ctx.Context, options)
			if err != nil {
				return fmt.Errorf("search images: %w", err)
			}
			return renderResult(ctx.Command, rootOptions.Output, pack)
		},
	))
	searchCmd.AddCommand(newSearchKindCommand(
		"videos",
		rootOptions,
		deps,
		func(ctx commandCallContext, options websearch.Options) error {
			pack, err := deps.WebSearch.SearchVideos(ctx.Context, options)
			if err != nil {
				return fmt.Errorf("search videos: %w", err)
			}
			return renderResult(ctx.Command, rootOptions.Output, pack)
		},
	))
	root.AddCommand(searchCmd)
}

type commandCallContext struct {
	Command *cobra.Command
	Context context.Context
}

func newSearchKindCommand(
	name string,
	rootOptions *rootFlags,
	_ *Dependencies,
	run func(commandCallContext, websearch.Options) error,
) *cobra.Command {
	options := searchFlags{}
	cmd := &cobra.Command{
		Use:   name + " <query>",
		Short: "Search " + name,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New(name + " search accepts exactly one query"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()

			count := options.Count
			if !cmd.Flags().Changed("count") {
				count = changedMaxResults(cmd, rootOptions)
			}
			return run(commandCallContext{Command: cmd, Context: ctx}, websearch.Options{
				Query:      args[0],
				Count:      count,
				Country:    options.Country,
				Language:   options.Language,
				SafeSearch: options.SafeSearch,
				Offset:     options.Offset,
			})
		},
	}
	cmd.Flags().IntVar(&options.Count, "count", 0, "search result count")
	cmd.Flags().StringVar(&options.Country, "country", "", "search country code")
	cmd.Flags().StringVar(&options.Language, "lang", "", "search language code")
	cmd.Flags().StringVar(&options.SafeSearch, "safe", "", "safe-search mode")
	cmd.Flags().IntVar(&options.Offset, "offset", 0, "search result offset")
	return cmd
}

func addFetchCommand(root *cobra.Command, rootOptions *rootFlags, deps *Dependencies) {
	options := fetchFlags{}
	cmd := &cobra.Command{
		Use:   "fetch <url>",
		Short: "Fetch one URL",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("fetch accepts exactly one URL"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()

			pack, err := deps.WebFetch.Fetch(ctx, webfetch.Request{
				URL:         args[0],
				UseBrowser:  options.UseBrowser,
				IncludeText: options.IncludeText,
				MaxBytes:    rootOptions.Runtime.Limits.MaxBytes,
				Timeout:     requestTimeout(cmd, rootOptions),
			})
			if err != nil {
				return fmt.Errorf("fetch URL: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, pack)
		},
	}
	cmd.Flags().BoolVar(&options.UseBrowser, "browser", false, "fetch with browser rendering")
	cmd.Flags().BoolVar(&options.IncludeText, "text", false, "include extracted text")
	root.AddCommand(cmd)
}
