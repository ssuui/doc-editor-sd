package auth

import (
	"sync"
	"time"
)

type Session struct {
	Token    string
	ExpireTs int64
}

var (
	globalSession *Session
	mu            sync.RWMutex
)

func SetNewSession(token string, expireTs int64) {
	mu.Lock()
	defer mu.Unlock()
	globalSession = &Session{Token: token, ExpireTs: expireTs}
}

func IsValidToken(token string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return isValidLocked(token)
}

func HasValidSession() bool {
	mu.RLock()
	defer mu.RUnlock()
	return isValidLocked(globalSessionToken())
}

func ClearSession() {
	mu.Lock()
	defer mu.Unlock()
	globalSession = nil
}

func StartCleanup() {
	ticker := time.NewTicker(time.Hour)
	go func() {
		for range ticker.C {
			mu.Lock()
			if globalSession != nil && globalSession.ExpireTs <= time.Now().Unix() {
				globalSession = nil
			}
			mu.Unlock()
		}
	}()
}

func globalSessionToken() string {
	if globalSession == nil {
		return ""
	}
	return globalSession.Token
}

func isValidLocked(token string) bool {
	if globalSession == nil || token == "" {
		return false
	}
	return globalSession.Token == token && globalSession.ExpireTs > time.Now().Unix()
}
