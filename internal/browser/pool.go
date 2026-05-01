package browser

import (
	"context"
	"errors"
	"runtime"
	"sync"

	"github.com/alnah/moth/internal/content"
)

const (
	minPoolSize = 1
	maxPoolSize = 4
	cpuDivisor  = 2
)

// Pool owns a bounded set of lazy browser workers.
type Pool struct {
	size      int
	config    poolConfig
	workers   []Worker
	available []Worker
	domains   *domainScheduler
	closedCh  chan struct{}
	waitCh    chan struct{}

	mu       sync.Mutex
	created  int
	closed   bool
	sessions map[string]*poolSession
}

// ResolvePoolSize clamps explicit or automatic worker counts to Moth browser bounds.
func ResolvePoolSize(workers int) int {
	if workers < 0 {
		return minPoolSize
	}
	if workers > 0 {
		return min(max(workers, minPoolSize), maxPoolSize)
	}

	return min(max(runtime.GOMAXPROCS(0)/cpuDivisor, minPoolSize), maxPoolSize)
}

// NewPool creates a lazy browser worker pool.
func NewPool(size int, opts ...Option) *Pool {
	config := defaultPoolConfig()
	for _, opt := range opts {
		opt(&config)
	}
	if config.workerFactory == nil {
		config.workerFactory = func(ctx context.Context) (Worker, error) {
			if config.configuredWorkerFactory != nil {
				return config.configuredWorkerFactory(ctx, config.workerOptions())
			}
			return newRodWorker(ctx, config)
		}
	}

	resolvedSize := ResolvePoolSize(size)
	return &Pool{
		size:      resolvedSize,
		config:    config,
		workers:   make([]Worker, 0, resolvedSize),
		available: make([]Worker, 0, resolvedSize),
		domains:   newDomainScheduler(),
		closedCh:  make(chan struct{}),
		waitCh:    make(chan struct{}),
		sessions:  make(map[string]*poolSession),
	}
}

// Acquire returns an existing worker or lazily creates one until the pool is full.
func (pool *Pool) Acquire(ctx context.Context) (Worker, error) {
	for {
		worker, canCreate, waitCh, err := pool.reserveWorkerSlot()
		if err != nil || worker != nil {
			return worker, err
		}
		if canCreate {
			return pool.createWorker(ctx)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-pool.closedCh:
			return nil, ErrPoolClosed
		case <-waitCh:
		}
	}
}

// Release returns a worker to the pool. It is safe after Close.
func (pool *Pool) Release(worker Worker) {
	if worker == nil {
		return
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.closed {
		return
	}
	pool.available = append(pool.available, worker)
	pool.notifyWaitersLocked()
}

// Close closes all workers exactly once and joins worker close errors.
func (pool *Pool) Close() error {
	pool.mu.Lock()
	if pool.closed {
		pool.mu.Unlock()
		return nil
	}
	pool.closed = true
	close(pool.closedCh)
	workers := append([]Worker(nil), pool.workers...)
	pool.mu.Unlock()

	errs := make([]error, 0, len(workers))
	for _, worker := range workers {
		if worker == nil {
			continue
		}
		if err := worker.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// FetchPage serializes work by registrable domain and extracts normalized page content.
func (pool *Pool) FetchPage(ctx context.Context, request PageRequest) (content.Item, error) {
	var item content.Item
	err := pool.withDomainWorker(ctx, request.URL, func(worker Worker) error {
		loadedPage, err := worker.OpenPage(ctx, request)
		if err != nil {
			return err
		}
		item, err = extractPageItem(loadedPage)
		return err
	})
	return item, err
}

// Screenshot serializes work by registrable domain and writes a rendered screenshot.
func (pool *Pool) Screenshot(ctx context.Context, request ScreenshotRequest) error {
	return pool.withDomainWorker(ctx, request.URL, func(worker Worker) error {
		image, err := worker.CaptureScreenshot(ctx, request)
		if err != nil {
			return err
		}
		return writeBrowserFile(request.Path, image, "screenshot")
	})
}

// OpenPage opens a persistent browser page and makes it active for its session.
func (pool *Pool) OpenPage(ctx context.Context, request OpenPageRequest) (PageInfo, error) {
	releaseDomain, err := pool.domains.acquire(ctx, request.URL)
	if err != nil {
		return PageInfo{}, err
	}
	defer releaseDomain()

	var page PageInfo
	err = pool.withPersistentSessionWorker(ctx, SessionRequest{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
	}, true, func(worker persistentPageWorker) error {
		var workerErr error
		page, workerErr = worker.OpenPersistentPage(ctx, request)
		return workerErr
	})
	return page, err
}

// ListPages lists persistent pages for a named session.
func (pool *Pool) ListPages(ctx context.Context, request SessionRequest) ([]PageInfo, error) {
	if _, ok := pool.existingSession(sessionKey(request.ProfileName, request.SessionName)); !ok {
		return []PageInfo{}, nil
	}

	var pages []PageInfo
	err := pool.withPersistentSessionWorker(ctx, request, false, func(worker persistentPageWorker) error {
		var err error
		pages, err = worker.ListPersistentPages(ctx, request)
		return err
	})
	return pages, err
}

// SwitchPage selects the active persistent page for a named session.
func (pool *Pool) SwitchPage(ctx context.Context, request PageSelection) (PageInfo, error) {
	var page PageInfo
	err := pool.withPersistentSessionWorker(
		ctx,
		selectionSession(request),
		false,
		func(worker persistentPageWorker) error {
			var err error
			page, err = worker.SwitchPersistentPage(ctx, request)
			return err
		},
	)
	return page, err
}

// ClosePage closes a persistent page and updates session active-page state.
func (pool *Pool) ClosePage(ctx context.Context, request PageSelection) error {
	key := sessionKey(request.ProfileName, request.SessionName)
	return pool.withPersistentSessionWorker(
		ctx,
		selectionSession(request),
		false,
		func(worker persistentPageWorker) error {
			if err := worker.ClosePersistentPage(ctx, request); err != nil {
				return err
			}
			session, ok := pool.existingSession(key)
			if !ok {
				return nil
			}
			return pool.removeSessionIfEmpty(ctx, key, session)
		},
	)
}

// Input types text into a selected or active persistent page element.
func (pool *Pool) Input(ctx context.Context, request InputRequest) error {
	return pool.withInteractiveSessionWorker(ctx, inputSession(request), func(worker interactiveWorker) error {
		return worker.Input(ctx, request)
	})
}

// Click clicks a selected or active persistent page element.
func (pool *Pool) Click(ctx context.Context, request InteractionRequest) error {
	return pool.withInteractiveSessionWorker(ctx, interactionSession(request), func(worker interactiveWorker) error {
		return worker.Click(ctx, request)
	})
}

// Wait waits for a condition on a selected or active persistent page element.
func (pool *Pool) Wait(ctx context.Context, request WaitRequest) error {
	return pool.withInteractiveSessionWorker(ctx, waitSession(request), func(worker interactiveWorker) error {
		return worker.Wait(ctx, request)
	})
}

// AccessibilityTree extracts stable accessibility nodes from a persistent page.
func (pool *Pool) AccessibilityTree(ctx context.Context, request AccessibilityRequest) (AccessibilityTree, error) {
	var tree AccessibilityTree
	err := pool.withAccessibleSessionWorker(ctx, accessibilitySession(request), func(worker accessibleWorker) error {
		var err error
		tree, err = worker.AccessibilityTree(ctx, request)
		return err
	})
	return tree, err
}

// Download captures browser-triggered download bytes to caller path.
func (pool *Pool) Download(ctx context.Context, request DownloadRequest) (CapturedDownload, error) {
	var result CapturedDownload
	err := pool.withDownloadSessionWorker(ctx, downloadSession(request), func(worker downloadWorker) error {
		captured, err := worker.CaptureDownload(ctx, request)
		if err != nil {
			return err
		}
		data, err := downloadBytes(captured.Bytes)
		if err != nil {
			return err
		}
		if err := writeBrowserFile(request.Path, data, "download"); err != nil {
			return err
		}
		result = CapturedDownload{Path: request.Path, Bytes: int64(len(data)), ContentType: captured.ContentType}
		return nil
	})
	return result, err
}

// ResponseMetadata captures bounded network response metadata.
func (pool *Pool) ResponseMetadata(ctx context.Context, request ResponseMetadataRequest) (ResponseMetadata, error) {
	var metadata ResponseMetadata
	err := pool.withDomainWorker(ctx, request.URL, func(worker Worker) error {
		metadataWorker, err := requireMetadataWorker(worker)
		if err != nil {
			return err
		}
		metadata, err = metadataWorker.ResponseMetadata(ctx, request)
		if err != nil {
			return err
		}
		metadata = normalizeResponseMetadata(metadata, request)
		return nil
	})
	return metadata, err
}

// PDF captures a rendered page as PDF bytes to caller path.
func (pool *Pool) PDF(ctx context.Context, request PDFRequest) error {
	return pool.withDomainWorker(ctx, request.URL, func(worker Worker) error {
		pdfWorker, err := requirePDFWorker(worker)
		if err != nil {
			return err
		}
		pdf, err := pdfWorker.CapturePDF(ctx, request)
		if err != nil {
			return err
		}
		return writeBrowserFile(request.Path, pdf, "pdf")
	})
}

// DetectManualChallenge reports manual-required CAPTCHA/login state without solving it.
func (pool *Pool) DetectManualChallenge(
	ctx context.Context,
	request ManualChallengeRequest,
) (ManualChallengeResult, error) {
	var result ManualChallengeResult
	err := pool.withManualChallengeSessionWorker(ctx, challengeSession(request), func(worker manualChallengeWorker) error {
		var err error
		result, err = worker.DetectManualChallenge(ctx, request)
		if err != nil {
			return err
		}
		result.Solved = false
		return nil
	})
	return result, err
}

func (pool *Pool) reserveWorkerSlot() (Worker, bool, <-chan struct{}, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	if pool.closed {
		return nil, false, nil, ErrPoolClosed
	}
	if len(pool.available) > 0 {
		last := len(pool.available) - 1
		worker := pool.available[last]
		pool.available = pool.available[:last]
		return worker, false, nil, nil
	}
	if pool.created >= pool.size {
		return nil, false, pool.waitCh, nil
	}
	pool.created++
	return nil, true, nil, nil
}

func (pool *Pool) createWorker(ctx context.Context) (Worker, error) {
	worker, err := pool.config.workerFactory(ctx)
	if err != nil {
		pool.releaseReservedSlot()
		return nil, err
	}
	if worker == nil {
		pool.releaseReservedSlot()
		return nil, errors.New("browser worker factory returned nil")
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.closed {
		if err := worker.Close(); err != nil {
			return nil, err
		}
		return nil, ErrPoolClosed
	}
	pool.workers = append(pool.workers, worker)
	return worker, nil
}

func (pool *Pool) releaseReservedSlot() {
	pool.mu.Lock()
	pool.created--
	pool.notifyWaitersLocked()
	pool.mu.Unlock()
}

func (pool *Pool) notifyWaitersLocked() {
	close(pool.waitCh)
	pool.waitCh = make(chan struct{})
}
