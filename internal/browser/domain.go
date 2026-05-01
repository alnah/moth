package browser

import (
	"context"
	"net"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/net/publicsuffix"
)

type domainScheduler struct {
	mu      sync.Mutex
	active  map[string]bool
	waiters map[string][]chan struct{}
}

func newDomainScheduler() *domainScheduler {
	return &domainScheduler{
		active:  make(map[string]bool),
		waiters: make(map[string][]chan struct{}),
	}
}

func (scheduler *domainScheduler) acquire(ctx context.Context, rawURL string) (func(), error) {
	key := registrableDomainKey(rawURL)
	for {
		scheduler.mu.Lock()
		if !scheduler.active[key] {
			scheduler.active[key] = true
			scheduler.mu.Unlock()
			return func() { scheduler.release(key) }, nil
		}

		grant := make(chan struct{})
		scheduler.waiters[key] = append(scheduler.waiters[key], grant)
		scheduler.mu.Unlock()

		select {
		case <-grant:
			return func() { scheduler.release(key) }, nil
		case <-ctx.Done():
			if scheduler.removeWaiter(key, grant) {
				return nil, ctx.Err()
			}
			return func() { scheduler.release(key) }, nil
		}
	}
}

func (scheduler *domainScheduler) release(key string) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()

	waiters := scheduler.waiters[key]
	if len(waiters) == 0 {
		delete(scheduler.active, key)
		return
	}

	grant := waiters[0]
	remaining := waiters[1:]
	if len(remaining) == 0 {
		delete(scheduler.waiters, key)
	} else {
		scheduler.waiters[key] = remaining
	}
	close(grant)
}

func (scheduler *domainScheduler) removeWaiter(key string, grant chan struct{}) bool {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()

	waiters := scheduler.waiters[key]
	for index, waiter := range waiters {
		if waiter != grant {
			continue
		}
		waiters = append(waiters[:index], waiters[index+1:]...)
		if len(waiters) == 0 {
			delete(scheduler.waiters, key)
		} else {
			scheduler.waiters[key] = waiters
		}
		return true
	}
	return false
}

func registrableDomainKey(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	host := strings.ToLower(strings.TrimSuffix(parsedURL.Hostname(), "."))
	if host == "" {
		return rawURL
	}
	if net.ParseIP(host) != nil {
		return host
	}

	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host
	}
	return domain
}
