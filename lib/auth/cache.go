package auth

import (
	"crypto/sha256"
	"sync"
	"time"
)

// SessionCacheTTL is how long a verified session is remembered.
//
// Checking a session costs two calls to Supabase (verify the token, then
// confirm the user is an admin). The dashboard makes several requests per
// page, so a warm server instance remembers the answer for a short time and
// skips those calls. The trade-off: a removed admin or a token revoked from
// elsewhere can keep working on an already-warm instance for up to this
// long. Logging out through the dashboard clears it right away on the
// instance that handles the logout, and always clears the browser's cookies.
var SessionCacheTTL = 30 * time.Second

const sessionCacheMax = 1000

type cachedSession struct {
	admin   Admin
	expires time.Time
}

var sessionCache = struct {
	sync.Mutex
	m map[[32]byte]cachedSession
}{m: map[[32]byte]cachedSession{}}

// The cache is keyed by a hash of the token, so tokens themselves aren't
// kept in memory longer than the request.
func cacheKey(token string) [32]byte { return sha256.Sum256([]byte(token)) }

func cacheGet(token string, now time.Time) (Admin, bool) {
	if SessionCacheTTL <= 0 || token == "" {
		return Admin{}, false
	}
	sessionCache.Lock()
	defer sessionCache.Unlock()
	e, ok := sessionCache.m[cacheKey(token)]
	if !ok || now.After(e.expires) {
		return Admin{}, false
	}
	return e.admin, true
}

func cachePut(token string, admin Admin, now time.Time) {
	if SessionCacheTTL <= 0 || token == "" {
		return
	}
	sessionCache.Lock()
	defer sessionCache.Unlock()
	if len(sessionCache.m) >= sessionCacheMax {
		for k, e := range sessionCache.m {
			if now.After(e.expires) {
				delete(sessionCache.m, k)
			}
		}
		if len(sessionCache.m) >= sessionCacheMax {
			sessionCache.m = map[[32]byte]cachedSession{}
		}
	}
	sessionCache.m[cacheKey(token)] = cachedSession{admin: admin, expires: now.Add(SessionCacheTTL)}
}

func cacheDelete(token string) {
	if token == "" {
		return
	}
	sessionCache.Lock()
	defer sessionCache.Unlock()
	delete(sessionCache.m, cacheKey(token))
}
