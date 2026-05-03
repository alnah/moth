package cli

import (
	"context"
	"runtime"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
	"github.com/alnah/moth/internal/limits"
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
	Start(context.Context, browser.StartRequest) (browser.BrowserStatus, error)
	Stop(context.Context, browser.StopRequest) (browser.BrowserStatus, error)
	Status(context.Context, browser.StatusRequest) (browser.BrowserStatus, error)
	Connect(context.Context, browser.ConnectRequest) (browser.BrowserStatus, error)
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

type defaultDependencyOptions struct {
	Limits     limits.Options
	BrowserBin string
}

type defaultDependencySet struct {
	Dependencies
	browserPool *browser.Pool
}

type defaultDependencyRuntime struct {
	options *rootFlags
	set     defaultDependencySet
}

func newDefaultDependencyRuntime(options *rootFlags) *defaultDependencyRuntime {
	return &defaultDependencyRuntime{options: options}
}

func (runtime *defaultDependencyRuntime) fill(deps *Dependencies) {
	if dependenciesComplete(*deps) {
		return
	}

	runtime.set = defaultDependencyFactory(defaultDependencyOptions{
		Limits:     runtime.options.Runtime.Limits,
		BrowserBin: runtime.options.Runtime.BrowserBin,
	})
	if deps.WebSearch == nil {
		deps.WebSearch = runtime.set.WebSearch
	}
	if deps.WebFetch == nil {
		deps.WebFetch = runtime.set.WebFetch
	}
	if deps.YouTube == nil {
		deps.YouTube = runtime.set.YouTube
	}
	if deps.YTDLP == nil {
		deps.YTDLP = runtime.set.YTDLP
	}
	if deps.Podcast == nil {
		deps.Podcast = runtime.set.Podcast
	}
	if deps.PodcastAudio == nil {
		deps.PodcastAudio = runtime.set.PodcastAudio
	}
	if deps.X == nil {
		deps.X = runtime.set.X
	}
	if deps.PDF2Text == nil {
		deps.PDF2Text = runtime.set.PDF2Text
	}
	if deps.Transcription == nil {
		deps.Transcription = runtime.set.Transcription
	}
	if deps.Tools == nil {
		deps.Tools = runtime.set.Tools
	}
	if deps.Browser == nil {
		deps.Browser = runtime.set.Browser
	}
}

func (runtime *defaultDependencyRuntime) closeBrowserPool() error {
	if runtime.set.browserPool == nil {
		return nil
	}
	return runtime.set.browserPool.Close()
}

var defaultDependencyFactory = defaultDependencies

func defaultDependencies(options defaultDependencyOptions) defaultDependencySet {
	credentials, environmentSettings, _ := config.LoadFromEnv(nil)
	retryingHTTPClient := defaultRetryingHTTPClient(options.Limits)
	browserBin := options.BrowserBin
	if browserBin == "" {
		browserBin = environmentSettings.RodBrowserBin
	}
	if browserBin == "" {
		resolved, err := tools.ResolveBrowser(context.Background(), tools.BrowserDoctorOptions{
			SearchCommonInstallPaths: true,
		}, tools.Platform{OS: runtime.GOOS})
		if err == nil {
			browserBin = resolved.Path
		}
	}
	browserPool := browser.NewPool(browser.ResolvePoolSize(0), browser.WithBrowserBin(browserBin))
	browserService := browser.NewPersistentService(browser.PersistentServiceOptions{
		BrowserBin: browserBin,
		Stateless:  browserPool,
	})

	return defaultDependencySet{
		Dependencies: Dependencies{
			WebSearch: websearch.NewClient(websearch.Config{
				Credentials: credentials,
				HTTPClient:  retryingHTTPClient,
			}),
			WebFetch: webfetch.New(webfetch.Options{BrowserFetcher: poolBrowserFetcher{pool: browserPool}}),
			YouTube: youtube.NewClient(youtube.Config{
				Credentials: credentials,
				HTTPClient:  retryingHTTPClient,
			}),
			YTDLP: ytdlp.New(ytdlp.Config{}),
			Podcast: podcast.NewClient(podcast.Config{
				Credentials: credentials,
				HTTPClient:  retryingHTTPClient,
			}),
			PodcastAudio: podcast.NewAudioDownloader(podcast.AudioDownloaderConfig{}),
			X: xclient.NewClient(xclient.Config{
				Credentials: credentials,
				HTTPClient:  retryingHTTPClient,
			}),
			PDF2Text: pdf2TextAdapter{},
			Transcription: transcription.NewClient(transcription.Config{
				Credentials: credentials,
				HTTPClient:  retryingHTTPClient,
			}),
			Tools:   toolsAdapter{},
			Browser: browserService,
		},
		browserPool: browserPool,
	}
}

func defaultRetryingHTTPClient(options limits.Options) *httpclient.Client {
	httpOptions := httpclient.Options{}
	if options.Retries > 0 {
		httpOptions.Attempts = options.Retries + 1
	}
	if options.RetryBase != limits.DefaultRetryBase {
		httpOptions.RetryBase = options.RetryBase
	}
	if options.RetryMax != limits.DefaultRetryMax {
		httpOptions.RetryMax = options.RetryMax
	}
	return httpclient.New(httpOptions)
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
