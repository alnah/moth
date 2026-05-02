package cli

import (
	"context"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/pdf2txt"
	"github.com/alnah/moth/internal/podcast"
	"github.com/alnah/moth/internal/tools"
	"github.com/alnah/moth/internal/transcription"
	"github.com/alnah/moth/internal/webfetch"
	"github.com/alnah/moth/internal/websearch"
	xclient "github.com/alnah/moth/internal/x"
	"github.com/alnah/moth/internal/youtube"
	"github.com/alnah/moth/internal/ytdlp"
)

// Dependencies contains the narrow services the CLI orchestrates.
type Dependencies struct {
	WebSearch     WebSearchService
	WebFetch      WebFetchService
	YouTube       YouTubeService
	YTDLP         YTDLPService
	Podcast       PodcastService
	PodcastAudio  PodcastAudioService
	X             XService
	PDF2Text      PDF2TextService
	Transcription TranscriptionService
	Tools         ToolsService
	Browser       BrowserService
}

// WebSearchService searches web, image, and video indexes.
type WebSearchService interface {
	SearchWeb(context.Context, websearch.Options) (content.Pack, error)
	SearchImages(context.Context, websearch.Options) (content.Pack, error)
	SearchVideos(context.Context, websearch.Options) (content.Pack, error)
}

// WebFetchService fetches one URL into a normalized content pack.
type WebFetchService interface {
	Fetch(context.Context, webfetch.Request) (content.Pack, error)
}

// YouTubeService exposes YouTube Data API content commands.
type YouTubeService interface {
	SearchVideos(context.Context, youtube.SearchOptions) (content.Pack, error)
	VideoDetails(context.Context, youtube.VideoDetailsOptions) (content.Pack, error)
}

// YTDLPService exposes yt-dlp acquisition commands.
type YTDLPService interface {
	Metadata(context.Context, ytdlp.MetadataRequest) (content.Item, error)
	DownloadSubtitles(context.Context, ytdlp.SubtitleRequest) (ytdlp.SubtitleFiles, error)
	ExtractAudio(context.Context, ytdlp.AudioRequest) (ytdlp.AudioFile, error)
}

// PodcastService exposes Podcast Index discovery commands.
type PodcastService interface {
	Search(context.Context, podcast.SearchOptions) (content.Pack, error)
	EpisodesByFeedID(context.Context, podcast.EpisodesByFeedIDOptions) (content.Pack, error)
}

// PodcastAudioService downloads podcast episode audio from feeds.
type PodcastAudioService interface {
	DownloadEpisodeAudio(context.Context, podcast.AudioDownloadOptions) (podcast.AudioFile, error)
}

// XService exposes bounded X API commands.
type XService interface {
	SearchRecent(context.Context, xclient.SearchOptions) (content.Pack, error)
	LookupPost(context.Context, xclient.LookupPostOptions) (content.Pack, error)
	UserPosts(context.Context, xclient.UserPostsOptions) (content.Pack, error)
}

// PDF2TextService extracts text from PDFs.
type PDF2TextService interface {
	Extract(context.Context, string, pdf2txt.Options) (content.Item, error)
}

// TranscriptionService transcribes one audio file.
type TranscriptionService interface {
	Transcribe(context.Context, transcription.Request) (transcription.Result, error)
}

// ToolsService reports external tool status.
type ToolsService interface {
	Doctor(context.Context, tools.DoctorOptions) (tools.DoctorReport, error)
}

// BrowserService exposes stable browser capabilities without Rod details.
type BrowserService interface {
	OpenPage(context.Context, browser.OpenPageRequest) (browser.PageInfo, error)
	ListPages(context.Context, browser.SessionRequest) ([]browser.PageInfo, error)
	SwitchPage(context.Context, browser.PageSelection) (browser.PageInfo, error)
	ClosePage(context.Context, browser.PageSelection) error
	Click(context.Context, browser.InteractionRequest) error
	Input(context.Context, browser.InputRequest) error
	Wait(context.Context, browser.WaitRequest) error
	ResponseMetadata(context.Context, browser.ResponseMetadataRequest) (browser.ResponseMetadata, error)
	AccessibilityTree(context.Context, browser.AccessibilityRequest) (browser.AccessibilityTree, error)
	DetectManualChallenge(context.Context, browser.ManualChallengeRequest) (browser.ManualChallengeResult, error)
	Screenshot(context.Context, browser.ScreenshotRequest) error
	PDF(context.Context, browser.PDFRequest) error
	Download(context.Context, browser.DownloadRequest) (browser.CapturedDownload, error)
}

func defaultDependencies() Dependencies {
	settings, _ := config.LoadFromEnv(nil)
	browserPool := browser.NewPool(browser.ResolvePoolSize(0), browser.WithBrowserBin(settings.RodBrowserBin))

	return Dependencies{
		WebSearch:     websearch.NewClient(websearch.Config{Settings: settings}),
		WebFetch:      webfetch.New(webfetch.Options{BrowserFetcher: poolBrowserFetcher{pool: browserPool}}),
		YouTube:       youtube.NewClient(youtube.Config{Settings: settings}),
		YTDLP:         ytdlp.New(ytdlp.Config{}),
		Podcast:       podcast.NewClient(podcast.Config{Settings: settings}),
		PodcastAudio:  podcast.NewAudioDownloader(podcast.AudioDownloaderConfig{}),
		X:             xclient.NewClient(xclient.Config{Settings: settings}),
		PDF2Text:      pdf2TextAdapter{},
		Transcription: transcription.NewClient(transcription.Config{Settings: settings}),
		Tools:         toolsAdapter{},
		Browser:       browserPool,
	}
}

func fillDefaultDependencies(deps Dependencies) Dependencies {
	if dependenciesComplete(deps) {
		return deps
	}

	defaults := defaultDependencies()
	if deps.WebSearch == nil {
		deps.WebSearch = defaults.WebSearch
	}
	if deps.WebFetch == nil {
		deps.WebFetch = defaults.WebFetch
	}
	if deps.YouTube == nil {
		deps.YouTube = defaults.YouTube
	}
	if deps.YTDLP == nil {
		deps.YTDLP = defaults.YTDLP
	}
	if deps.Podcast == nil {
		deps.Podcast = defaults.Podcast
	}
	if deps.PodcastAudio == nil {
		deps.PodcastAudio = defaults.PodcastAudio
	}
	if deps.X == nil {
		deps.X = defaults.X
	}
	if deps.PDF2Text == nil {
		deps.PDF2Text = defaults.PDF2Text
	}
	if deps.Transcription == nil {
		deps.Transcription = defaults.Transcription
	}
	if deps.Tools == nil {
		deps.Tools = defaults.Tools
	}
	if deps.Browser == nil {
		deps.Browser = defaults.Browser
	}
	return deps
}

func dependenciesComplete(deps Dependencies) bool {
	return deps.WebSearch != nil &&
		deps.WebFetch != nil &&
		deps.YouTube != nil &&
		deps.YTDLP != nil &&
		deps.Podcast != nil &&
		deps.PodcastAudio != nil &&
		deps.X != nil &&
		deps.PDF2Text != nil &&
		deps.Transcription != nil &&
		deps.Tools != nil &&
		deps.Browser != nil
}

type poolBrowserFetcher struct {
	pool *browser.Pool
}

func (fetcher poolBrowserFetcher) FetchRenderedPage(
	ctx context.Context,
	request webfetch.BrowserRequest,
) (webfetch.BrowserResponse, error) {
	if request.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, request.Timeout)
		defer cancel()
	}

	loadedPage, err := fetcher.pool.FetchRenderedPage(ctx, browser.PageRequest{
		URL:      request.URL,
		MaxBytes: request.MaxBytes,
	})
	if err != nil {
		return webfetch.BrowserResponse{}, err
	}
	return webfetch.BrowserResponse{URL: loadedPage.URL, ContentType: "text/html", HTML: loadedPage.HTML}, nil
}

type pdf2TextAdapter struct{}

func (pdf2TextAdapter) Extract(ctx context.Context, inputPDF string, options pdf2txt.Options) (content.Item, error) {
	return pdf2txt.New(options).Extract(ctx, inputPDF)
}

type toolsAdapter struct{}

func (toolsAdapter) Doctor(ctx context.Context, options tools.DoctorOptions) (tools.DoctorReport, error) {
	return tools.Doctor(ctx, options)
}
