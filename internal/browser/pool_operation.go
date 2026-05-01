package browser

import "context"

func (pool *Pool) withDomainWorker(ctx context.Context, rawURL string, use func(Worker) error) error {
	releaseDomain, err := pool.domains.acquire(ctx, rawURL)
	if err != nil {
		return err
	}
	defer releaseDomain()

	worker, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer pool.Release(worker)
	return use(worker)
}

func (pool *Pool) withPersistentSessionWorker(
	ctx context.Context,
	request SessionRequest,
	create bool,
	use func(persistentPageWorker) error,
) error {
	return pool.withSessionWorker(ctx, request, create, func(worker Worker) error {
		persistentWorker, err := requirePersistentPageWorker(worker)
		if err != nil {
			return err
		}
		return use(persistentWorker)
	})
}

func (pool *Pool) withInteractiveSessionWorker(
	ctx context.Context,
	request SessionRequest,
	use func(interactiveWorker) error,
) error {
	return pool.withSessionWorker(ctx, request, false, func(worker Worker) error {
		interactive, err := requireInteractiveWorker(worker)
		if err != nil {
			return err
		}
		return use(interactive)
	})
}

func (pool *Pool) withAccessibleSessionWorker(
	ctx context.Context,
	request SessionRequest,
	use func(accessibleWorker) error,
) error {
	return pool.withSessionWorker(ctx, request, false, func(worker Worker) error {
		accessible, err := requireAccessibleWorker(worker)
		if err != nil {
			return err
		}
		return use(accessible)
	})
}

func (pool *Pool) withDownloadSessionWorker(
	ctx context.Context,
	request SessionRequest,
	use func(downloadWorker) error,
) error {
	return pool.withSessionWorker(ctx, request, false, func(worker Worker) error {
		downloads, err := requireDownloadWorker(worker)
		if err != nil {
			return err
		}
		return use(downloads)
	})
}

func (pool *Pool) withManualChallengeSessionWorker(
	ctx context.Context,
	request SessionRequest,
	use func(manualChallengeWorker) error,
) error {
	return pool.withSessionWorker(ctx, request, false, func(worker Worker) error {
		challenge, err := requireManualChallengeWorker(worker)
		if err != nil {
			return err
		}
		return use(challenge)
	})
}
