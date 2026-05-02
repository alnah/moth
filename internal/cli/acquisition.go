package cli

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/pdf2txt"
	"github.com/alnah/moth/internal/podcast"
	"github.com/alnah/moth/internal/transcription"
	"github.com/alnah/moth/internal/x"
	"github.com/alnah/moth/internal/youtube"
	"github.com/alnah/moth/internal/ytdlp"
)

func addYouTubeCommand(root *cobra.Command, rootOptions *rootFlags, deps Dependencies) {
	youtubeCmd := &cobra.Command{Use: "youtube", Short: "Search and acquire YouTube content"}

	var searchOptions struct {
		Region string
		Lang   string
		Safe   string
		Page   string
	}
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search YouTube videos",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("youtube search accepts exactly one query"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			pack, err := deps.YouTube.SearchVideos(ctx, youtube.SearchOptions{
				Query:             args[0],
				MaxResults:        changedMaxResults(cmd, rootOptions.Limits.MaxResults),
				RegionCode:        searchOptions.Region,
				RelevanceLanguage: searchOptions.Lang,
				SafeSearch:        searchOptions.Safe,
				PageToken:         searchOptions.Page,
			})
			if err != nil {
				return fmt.Errorf("youtube search: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, pack)
		},
	}
	searchCmd.Flags().StringVar(&searchOptions.Region, "region", "", "YouTube region code")
	searchCmd.Flags().StringVar(&searchOptions.Lang, "lang", "", "YouTube relevance language")
	searchCmd.Flags().StringVar(&searchOptions.Safe, "safe", "", "YouTube safe search")
	searchCmd.Flags().StringVar(&searchOptions.Page, "page-token", "", "YouTube page token")

	metadataCmd := &cobra.Command{
		Use:   "metadata <url-or-id>",
		Short: "Fetch YouTube video metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("youtube metadata accepts exactly one URL or ID"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			pack, err := deps.YouTube.VideoDetails(ctx, youtube.VideoDetailsOptions{IDs: []string{args[0]}})
			if err != nil {
				return fmt.Errorf("youtube metadata: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, pack)
		},
	}

	var subtitleOptions struct {
		OutputDir string
		Languages []string
		Format    string
		Automatic bool
	}
	subtitleCmd := &cobra.Command{
		Use:   "subtitles <url-or-id>",
		Short: "Download YouTube subtitles",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("youtube subtitles accepts exactly one URL or ID"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			files, err := deps.YTDLP.DownloadSubtitles(ctx, ytdlp.SubtitleRequest{
				URL:              args[0],
				OutputDir:        subtitleOptions.OutputDir,
				Languages:        subtitleOptions.Languages,
				Format:           subtitleOptions.Format,
				IncludeAutomatic: subtitleOptions.Automatic,
			})
			if err != nil {
				return fmt.Errorf("youtube subtitles: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, subtitlePack(files))
		},
	}
	subtitleCmd.Flags().StringVar(&subtitleOptions.OutputDir, "output-dir", "", "subtitle output directory")
	subtitleCmd.Flags().StringArrayVar(&subtitleOptions.Languages, "language", nil, "subtitle language")
	subtitleCmd.Flags().StringVar(&subtitleOptions.Format, "format", "", "subtitle format")
	subtitleCmd.Flags().BoolVar(&subtitleOptions.Automatic, "automatic", false, "include automatic subtitles")

	var audioOptions struct {
		OutputDir string
		Format    string
	}
	audioCmd := &cobra.Command{
		Use:   "audio <url-or-id>",
		Short: "Extract YouTube audio",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("youtube audio accepts exactly one URL or ID"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			file, err := deps.YTDLP.ExtractAudio(ctx, ytdlp.AudioRequest{
				URL:       args[0],
				OutputDir: audioOptions.OutputDir,
				Format:    audioOptions.Format,
			})
			if err != nil {
				return fmt.Errorf("youtube audio: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, audioPack(file))
		},
	}
	audioCmd.Flags().StringVar(&audioOptions.OutputDir, "output-dir", "", "audio output directory")
	audioCmd.Flags().StringVar(&audioOptions.Format, "format", "", "audio format")

	youtubeCmd.AddCommand(searchCmd, metadataCmd, subtitleCmd, audioCmd)
	root.AddCommand(youtubeCmd)
}

func addPodcastCommand(root *cobra.Command, rootOptions *rootFlags, deps Dependencies) {
	podcastCmd := &cobra.Command{Use: "podcast", Short: "Search and acquire podcast content"}

	var podcastSearchOptions struct {
		Clean    bool
		FullText bool
	}
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search podcasts",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("podcast search accepts exactly one query"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			pack, err := deps.Podcast.Search(ctx, podcast.SearchOptions{
				Query:      args[0],
				MaxResults: changedMaxResults(cmd, rootOptions.Limits.MaxResults),
				Clean:      podcastSearchOptions.Clean,
				FullText:   podcastSearchOptions.FullText,
			})
			if err != nil {
				return fmt.Errorf("podcast search: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, pack)
		},
	}
	searchCmd.Flags().BoolVar(&podcastSearchOptions.Clean, "clean", false, "request clean podcast index results")
	searchCmd.Flags().BoolVar(&podcastSearchOptions.FullText, "fulltext", false, "request full text podcast index search")

	var episodeOptions struct{ FullText bool }
	episodesCmd := &cobra.Command{
		Use:   "episodes <feed-id>",
		Short: "List podcast episodes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("podcast episodes accepts exactly one feed ID"))
			}
			feedID, err := strconv.Atoi(args[0])
			if err != nil {
				return newInvalidArgumentsError(fmt.Errorf("invalid feed ID: %w", err))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			pack, err := deps.Podcast.EpisodesByFeedID(ctx, podcast.EpisodesByFeedIDOptions{
				FeedID:     feedID,
				MaxResults: changedMaxResults(cmd, rootOptions.Limits.MaxResults),
				FullText:   episodeOptions.FullText,
			})
			if err != nil {
				return fmt.Errorf("podcast episodes: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, pack)
		},
	}
	episodesCmd.Flags().BoolVar(&episodeOptions.FullText, "fulltext", false, "request full episode text")

	var audioOptions struct{ ContentTypes []string }
	audioCmd := &cobra.Command{
		Use:   "audio <feed-url> <episode-guid>",
		Short: "Download podcast episode audio",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return newInvalidArgumentsError(errors.New("podcast audio accepts feed URL and episode GUID"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			file, err := deps.PodcastAudio.DownloadEpisodeAudio(ctx, podcast.AudioDownloadOptions{
				FeedURL:             args[0],
				EpisodeGUID:         args[1],
				AllowedContentTypes: audioOptions.ContentTypes,
				MaxBytes:            rootOptions.Limits.MaxBytes,
			})
			if err != nil {
				return fmt.Errorf("podcast audio: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, podcastAudioPack(file))
		},
	}
	audioCmd.Flags().StringArrayVar(&audioOptions.ContentTypes, "content-type", nil, "allowed audio content type")

	podcastCmd.AddCommand(searchCmd, episodesCmd, audioCmd)
	root.AddCommand(podcastCmd)
}

func addXCommand(root *cobra.Command, rootOptions *rootFlags, deps Dependencies) {
	xCmd := &cobra.Command{Use: "x", Short: "Search and fetch X posts"}

	var searchOptions struct {
		NextToken   string
		MaxRequests int
	}
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search recent X posts",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("x search accepts exactly one query"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			pack, err := deps.X.SearchRecent(ctx, x.SearchOptions{
				Query:       args[0],
				MaxResults:  changedMaxResults(cmd, rootOptions.Limits.MaxResults),
				MaxRequests: searchOptions.MaxRequests,
				NextToken:   searchOptions.NextToken,
			})
			if err != nil {
				return fmt.Errorf("x search: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, pack)
		},
	}
	searchCmd.Flags().StringVar(&searchOptions.NextToken, "next-token", "", "X pagination token")
	searchCmd.Flags().IntVar(&searchOptions.MaxRequests, "max-requests", 0, "maximum X requests")

	postCmd := &cobra.Command{
		Use:   "post <post-id>",
		Short: "Fetch one X post",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("x post accepts exactly one post ID"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			pack, err := deps.X.LookupPost(ctx, x.LookupPostOptions{ID: args[0]})
			if err != nil {
				return fmt.Errorf("x post: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, pack)
		},
	}

	var userOptions struct {
		NextToken   string
		MaxRequests int
	}
	userCmd := &cobra.Command{
		Use:   "user <user-id>",
		Short: "Fetch X user posts by user ID",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("x user accepts exactly one user ID"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			pack, err := deps.X.UserPosts(ctx, x.UserPostsOptions{
				UserID:      args[0],
				MaxResults:  changedMaxResults(cmd, rootOptions.Limits.MaxResults),
				MaxRequests: userOptions.MaxRequests,
				NextToken:   userOptions.NextToken,
			})
			if err != nil {
				return fmt.Errorf("x user: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, pack)
		},
	}
	userCmd.Flags().StringVar(&userOptions.NextToken, "next-token", "", "X pagination token")
	userCmd.Flags().IntVar(&userOptions.MaxRequests, "max-requests", 0, "maximum X requests")

	xCmd.AddCommand(searchCmd, postCmd, userCmd)
	root.AddCommand(xCmd)
}

func addPDF2TextCommand(root *cobra.Command, rootOptions *rootFlags, deps Dependencies) {
	var options struct {
		OCRAllowed  bool
		OCRDisabled bool
		OCRLanguage string
	}
	cmd := &cobra.Command{
		Use:   "pdf2txt <file-or-url>",
		Short: "Extract PDF text",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("pdf2txt accepts exactly one file or URL"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			ocrAllowed := options.OCRAllowed && !options.OCRDisabled
			item, err := deps.PDF2Text.Extract(ctx, args[0], pdf2txt.Options{
				OCRAllowed:   ocrAllowed,
				OCRLanguage:  options.OCRLanguage,
				MaxTextBytes: rootOptions.Limits.MaxBytes,
			})
			if err != nil {
				return fmt.Errorf("pdf2txt: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, contentPack(item))
		},
	}
	cmd.Flags().BoolVar(&options.OCRAllowed, "ocr", false, "allow OCR fallback")
	cmd.Flags().BoolVar(&options.OCRDisabled, "no-ocr", false, "disable OCR fallback")
	cmd.Flags().StringVar(&options.OCRLanguage, "ocr-language", "", "OCR language")
	root.AddCommand(cmd)
}

func addTranscribeCommand(root *cobra.Command, rootOptions *rootFlags, deps Dependencies) {
	var options struct {
		Language               string
		Model                  string
		ResponseFormat         string
		TimestampGranularities []string
	}
	cmd := &cobra.Command{
		Use:   "transcribe <file>",
		Short: "Transcribe one audio file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return newInvalidArgumentsError(errors.New("transcribe accepts exactly one file"))
			}
			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			result, err := deps.Transcription.Transcribe(ctx, transcription.Request{
				FilePath:               args[0],
				Language:               options.Language,
				Model:                  options.Model,
				ResponseFormat:         options.ResponseFormat,
				TimestampGranularities: options.TimestampGranularities,
			})
			if err != nil {
				return fmt.Errorf("transcribe: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, transcriptionPack(result))
		},
	}
	cmd.Flags().StringVar(&options.Language, "language", "", "transcription language")
	cmd.Flags().StringVar(&options.Model, "model", "", "OpenAI transcription model")
	cmd.Flags().StringVar(&options.ResponseFormat, "response-format", "", "OpenAI response format")
	cmd.Flags().StringArrayVar(&options.TimestampGranularities, "timestamp-granularity", nil, "timestamp granularity")
	root.AddCommand(cmd)
}

var _ = content.TypeContentPack
