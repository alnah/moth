package browser

import (
	"context"
	"errors"
	"sort"
	"time"
)

// Reap closes expired pages, sessions, and browsers.
func (service *PersistentService) Reap(ctx context.Context, now time.Time) (ReapReport, error) {
	var report ReapReport
	var errs []error

	for _, scope := range sortedBrowserScopes(service.browsers) {
		scopeReport, err := service.reapScope(ctx, scope, now)
		report.ClosedPages += scopeReport.ClosedPages
		report.ClosedSessions += scopeReport.ClosedSessions
		report.ClosedBrowsers += scopeReport.ClosedBrowsers
		if err != nil {
			errs = append(errs, err)
		}
	}
	return report, errors.Join(errs...)
}

func (service *PersistentService) reapScope(
	ctx context.Context,
	scope string,
	now time.Time,
) (ReapReport, error) {
	var report ReapReport
	record := service.browsers[scope]
	if record == nil {
		return report, nil
	}
	connection, ok := service.cachedConnection(scope)
	if !ok {
		return report, nil
	}
	state, err := service.loadState(scope)
	if err != nil || state.DebugURL != connection.DebugURL {
		service.invalidateConnection(scope)
		return report, nil //nolint:nilerr // Missing state invalidates the cache.
	}

	sessionReport, failed, sessionErr := service.reapSessions(ctx, connection.Connection, record, now)
	report.ClosedPages += sessionReport.ClosedPages
	report.ClosedSessions += sessionReport.ClosedSessions
	if failed {
		return report, sessionErr
	}
	if record.LastUsed.IsZero() || now.Sub(record.LastUsed) < service.lifecycle.BrowserIdle {
		return report, sessionErr
	}

	closed, closeErr := service.closeAllSessions(ctx, connection.Connection, record)
	report.ClosedPages += closed
	if closeErr != nil {
		return report, errors.Join(sessionErr, closeErr)
	}
	if state.Owned {
		if closeErr := connection.Connection.Close(ctx); closeErr != nil {
			return report, errors.Join(sessionErr, closeErr)
		}
		if removeErr := service.removeState(scope); removeErr != nil {
			return report, errors.Join(sessionErr, removeErr)
		}
	} else if disconnectErr := disconnectConnection(ctx, connection.Connection); disconnectErr != nil {
		return report, errors.Join(sessionErr, disconnectErr)
	}
	service.invalidateConnection(scope)
	service.deleteBrowser(scope)
	report.ClosedBrowsers++
	return report, sessionErr
}

func (service *PersistentService) reapSessions(
	ctx context.Context,
	connection BrowserConnection,
	record *browserRecord,
	now time.Time,
) (ReapReport, bool, error) {
	var report ReapReport
	var errs []error
	failed := false

	for _, key := range sortedSessionKeys(record.Sessions) {
		session, found, syncErr := service.syncSessionPages(ctx, record.Scope, connection, key, now)
		if syncErr != nil {
			errs = append(errs, syncErr)
			failed = true
			continue
		}
		if !found {
			continue
		}

		pageReport, pageErr := service.reapIdlePages(ctx, connection, key, session, now)
		report.ClosedPages += pageReport.ClosedPages
		if pageErr != nil {
			errs = append(errs, pageErr)
			failed = true
		}
		if now.Sub(session.LastUsed) < service.lifecycle.SessionIdle &&
			now.Sub(session.CreatedAt) < service.lifecycle.SessionMax {
			continue
		}

		closed, closeErr := service.closeSession(ctx, connection, key, session)
		report.ClosedPages += closed
		if closeErr != nil {
			errs = append(errs, closeErr)
			failed = true
			continue
		}
		delete(record.Sessions, key)
		report.ClosedSessions++
	}
	return report, failed, errors.Join(errs...)
}

func (service *PersistentService) reapIdlePages(
	ctx context.Context,
	connection BrowserConnection,
	key persistentSessionKey,
	session *sessionRecord,
	now time.Time,
) (ReapReport, error) {
	var report ReapReport
	var errs []error
	for _, pageID := range sortedPageIDs(session.Pages) {
		page := session.Pages[pageID]
		if page == nil || now.Sub(page.LastUsed) < service.lifecycle.PageIdle {
			continue
		}
		if closeErr := service.closePage(ctx, connection, key, pageID); closeErr != nil {
			errs = append(errs, closeErr)
			continue
		}
		report.ClosedPages++
	}
	return report, errors.Join(errs...)
}

func (service *PersistentService) closeSession(
	ctx context.Context,
	connection BrowserConnection,
	key persistentSessionKey,
	session *sessionRecord,
) (int, error) {
	closed := 0
	var errs []error
	for _, pageID := range sortedPageIDs(session.Pages) {
		if err := service.closePage(ctx, connection, key, pageID); err != nil {
			errs = append(errs, err)
			continue
		}
		closed++
	}
	return closed, errors.Join(errs...)
}

func (service *PersistentService) closeAllSessions(
	ctx context.Context,
	connection BrowserConnection,
	record *browserRecord,
) (int, error) {
	closed := 0
	var errs []error
	for _, key := range sortedSessionKeys(record.Sessions) {
		session := record.Sessions[key]
		count, err := service.closeSession(ctx, connection, key, session)
		closed += count
		if err != nil {
			errs = append(errs, err)
			continue
		}
		delete(record.Sessions, key)
	}
	return closed, errors.Join(errs...)
}

func sortedBrowserScopes(records map[string]*browserRecord) []string {
	scopes := make([]string, 0, len(records))
	for scope := range records {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	return scopes
}

func sortedSessionKeys(records map[persistentSessionKey]*sessionRecord) []persistentSessionKey {
	keys := make([]persistentSessionKey, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ProfileName != keys[j].ProfileName {
			return keys[i].ProfileName < keys[j].ProfileName
		}
		return keys[i].SessionName < keys[j].SessionName
	})
	return keys
}
