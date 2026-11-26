package browser

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Default persistent browser lifecycle limits.
const (
	DefaultBrowserIdle = 15 * time.Minute
	DefaultSessionIdle = 15 * time.Minute
	DefaultSessionMax  = 2 * time.Hour
	DefaultPageIdle    = 30 * time.Minute
	DefaultMaxPages    = 8
)

// LifecycleOptions configures persistent browser reaping.
type LifecycleOptions struct {
	BrowserIdle time.Duration
	SessionIdle time.Duration
	SessionMax  time.Duration
	PageIdle    time.Duration
	MaxPages    int
}

func (options LifecycleOptions) withDefaults() LifecycleOptions {
	if options.BrowserIdle <= 0 {
		options.BrowserIdle = DefaultBrowserIdle
	}
	if options.SessionIdle <= 0 {
		options.SessionIdle = DefaultSessionIdle
	}
	if options.SessionMax <= 0 {
		options.SessionMax = DefaultSessionMax
	}
	if options.PageIdle <= 0 {
		options.PageIdle = DefaultPageIdle
	}
	if options.MaxPages <= 0 {
		options.MaxPages = DefaultMaxPages
	}
	return options
}

// ReapReport counts resources closed by one reaping pass.
type ReapReport struct {
	ClosedPages    int
	ClosedSessions int
	ClosedBrowsers int
}

type cachedBrowserConnection struct {
	DebugURL   string
	Connection BrowserConnection
}

type persistentSessionKey struct {
	Scope       string
	ProfileName string
	SessionName string
}

func newPersistentSessionKey(scope string, profileName string, sessionName string) persistentSessionKey {
	return persistentSessionKey{
		Scope:       scope,
		ProfileName: profileName,
		SessionName: sessionName,
	}
}

type browserRecord struct {
	Scope     string
	Owned     bool
	CreatedAt time.Time
	LastUsed  time.Time
	Sessions  map[persistentSessionKey]*sessionRecord
}

type sessionRecord struct {
	CreatedAt time.Time
	LastUsed  time.Time
	Pages     map[string]*pageRecord
}

type pageRecord struct {
	URL       string
	CreatedAt time.Time
	LastUsed  time.Time
}

// ActiveBrowsers returns the number of tracked browser scopes.
func (service *PersistentService) ActiveBrowsers() int {
	return len(service.browsers)
}

func (service *PersistentService) ensureBrowser(scope string, now time.Time, owned bool) *browserRecord {
	record := service.browsers[scope]
	if record != nil {
		record.Owned = owned
		return record
	}
	record = &browserRecord{
		Scope:     scope,
		Owned:     owned,
		CreatedAt: now,
		LastUsed:  now,
		Sessions:  make(map[persistentSessionKey]*sessionRecord),
	}
	service.browsers[scope] = record
	return record
}

func (service *PersistentService) touchBrowser(scope string, now time.Time, owned bool) {
	record := service.ensureBrowser(scope, now, owned)
	record.LastUsed = now
}

func (service *PersistentService) resetBrowser(scope string, now time.Time, owned bool) {
	service.browsers[scope] = &browserRecord{
		Scope:     scope,
		Owned:     owned,
		CreatedAt: now,
		LastUsed:  now,
		Sessions:  make(map[persistentSessionKey]*sessionRecord),
	}
}

func (service *PersistentService) deleteBrowser(scope string) {
	delete(service.browsers, scope)
}

func (service *PersistentService) ensureSession(
	record *browserRecord,
	key persistentSessionKey,
	now time.Time,
) *sessionRecord {
	session := record.Sessions[key]
	if session != nil {
		return session
	}
	session = &sessionRecord{
		CreatedAt: now,
		LastUsed:  now,
		Pages:     make(map[string]*pageRecord),
	}
	record.Sessions[key] = session
	return session
}

func (service *PersistentService) syncSessionPages(
	ctx context.Context,
	scope string,
	connection BrowserConnection,
	key persistentSessionKey,
	now time.Time,
) (*sessionRecord, bool, error) {
	record := service.browsers[scope]
	if record == nil {
		return nil, false, nil
	}
	pages, err := connection.ListPages(ctx, SessionRequest{
		ProfileName: key.ProfileName,
		SessionName: key.SessionName,
	})
	if err != nil {
		return nil, false, err
	}
	if len(pages) == 0 {
		delete(record.Sessions, key)
		return nil, false, nil
	}
	session := service.ensureSession(record, key, now)
	seen := make(map[string]bool, len(pages))
	for _, page := range pages {
		seen[page.ID] = true
		entry := session.Pages[page.ID]
		if entry == nil {
			entry = &pageRecord{
				URL:       page.URL,
				CreatedAt: now,
				LastUsed:  now,
			}
			session.Pages[page.ID] = entry
			continue
		}
		entry.URL = page.URL
	}
	for pageID := range session.Pages {
		if !seen[pageID] {
			delete(session.Pages, pageID)
		}
	}
	return session, true, nil
}

func (service *PersistentService) recordPage(
	scope string,
	key persistentSessionKey,
	page PageInfo,
	now time.Time,
) {
	record := service.browsers[scope]
	if record == nil {
		return
	}
	session := service.ensureSession(record, key, now)
	entry := session.Pages[page.ID]
	if entry == nil {
		session.Pages[page.ID] = &pageRecord{
			URL:       page.URL,
			CreatedAt: now,
			LastUsed:  now,
		}
	} else {
		entry.URL = page.URL
		entry.LastUsed = now
	}
	session.LastUsed = now
	record.LastUsed = now
}

func (service *PersistentService) touchSession(scope string, key persistentSessionKey, now time.Time) {
	record := service.browsers[scope]
	if record == nil {
		return
	}
	session := service.ensureSession(record, key, now)
	session.LastUsed = now
	record.LastUsed = now
}

func (service *PersistentService) touchRequestPage(
	state persistentBrowserState,
	profileName string,
	sessionName string,
	pageID string,
) {
	service.touchPage(
		state.Scope,
		newPersistentSessionKey(state.Scope, profileName, sessionName),
		pageID,
		service.clock(),
	)
}

func (service *PersistentService) touchPage(
	scope string,
	key persistentSessionKey,
	pageID string,
	now time.Time,
) {
	service.touchSession(scope, key, now)
	record := service.browsers[scope]
	if record == nil {
		return
	}
	session := record.Sessions[key]
	if session == nil {
		return
	}
	page := session.Pages[pageID]
	if page != nil {
		page.LastUsed = now
	}
}

func (service *PersistentService) removePage(scope string, key persistentSessionKey, pageID string) {
	record := service.browsers[scope]
	if record == nil {
		return
	}
	session := record.Sessions[key]
	if session == nil {
		return
	}
	delete(session.Pages, pageID)
	if len(session.Pages) == 0 {
		delete(record.Sessions, key)
	}
}

func (service *PersistentService) enforcePageLimit(
	ctx context.Context,
	connection BrowserConnection,
	key persistentSessionKey,
	newPageID string,
	now time.Time,
) error {
	record := service.browsers[key.Scope]
	if record == nil {
		return nil
	}
	session := record.Sessions[key]
	if session == nil {
		return nil
	}
	for len(session.Pages) > service.lifecycle.MaxPages {
		oldest := ""
		for _, pageID := range sortedPageIDs(session.Pages) {
			if pageID != newPageID {
				oldest = pageID
				break
			}
		}
		if oldest == "" {
			return nil
		}
		if err := service.closePage(ctx, connection, key, oldest); err != nil {
			rollbackErr := service.closePage(ctx, connection, key, newPageID)
			return fmt.Errorf("enforce page limit: %w", errors.Join(err, rollbackErr))
		}
		session.LastUsed = now
		record.LastUsed = now
	}
	return nil
}

func (service *PersistentService) closePage(
	ctx context.Context,
	connection BrowserConnection,
	key persistentSessionKey,
	pageID string,
) error {
	if err := connection.ClosePage(ctx, PageSelection{
		ProfileName: key.ProfileName,
		SessionName: key.SessionName,
		PageID:      pageID,
	}); err != nil {
		return err
	}
	service.removePage(key.Scope, key, pageID)
	return nil
}

func (service *PersistentService) cachedConnection(scope string) (cachedBrowserConnection, bool) {
	connection, ok := service.connections[scope]
	return connection, ok
}

func (service *PersistentService) cacheConnection(
	scope string,
	debugURL string,
	connection BrowserConnection,
) {
	service.connections[scope] = cachedBrowserConnection{
		DebugURL:   debugURL,
		Connection: connection,
	}
}

func (service *PersistentService) connectAndCache(
	ctx context.Context,
	scope string,
	debugURL string,
) (BrowserConnection, error) {
	connection, err := service.connector.ConnectBrowser(ctx, debugURL)
	if err != nil {
		return nil, err
	}
	service.cacheConnection(scope, debugURL, connection)
	return connection, nil
}

func (service *PersistentService) connectionFor(
	ctx context.Context,
	scope string,
	debugURL string,
) (BrowserConnection, error) {
	if cached, ok := service.connections[scope]; ok && cached.DebugURL == debugURL {
		return cached.Connection, nil
	}
	return service.connectAndCache(ctx, scope, debugURL)
}

func (service *PersistentService) invalidateConnection(scope string) {
	delete(service.connections, scope)
}

func disconnectConnection(ctx context.Context, connection BrowserConnection) error {
	return connection.Disconnect(ctx)
}

func sortedPageIDs(pages map[string]*pageRecord) []string {
	ids := make([]string, 0, len(pages))
	for id := range pages {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := pages[ids[i]], pages[ids[j]]
		if !left.LastUsed.Equal(right.LastUsed) {
			return left.LastUsed.Before(right.LastUsed)
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return ids[i] < ids[j]
	})
	return ids
}
