package httpserver

import (
	"sync"
	"time"
)

const (
	loginFailureWindow = 15 * time.Minute
	loginAttemptTTL    = 30 * time.Minute
	maxLoginEntries    = 4096
)

type loginAttempt struct {
	failures     int
	firstFailure time.Time
	lastFailure  time.Time
	blockedUntil time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
	now      func() time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]loginAttempt), now: time.Now}
}

func (l *loginLimiter) retryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	attempt, found := l.attempts[key]
	if !found || !attempt.blockedUntil.After(now) {
		return 0
	}
	return attempt.blockedUntil.Sub(now)
}

func (l *loginLimiter) recordFailure(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	attempt := l.attempts[key]
	if attempt.firstFailure.IsZero() || now.Sub(attempt.firstFailure) > loginFailureWindow {
		attempt = loginAttempt{firstFailure: now}
	}
	attempt.failures++
	attempt.lastFailure = now

	var block time.Duration
	if attempt.failures >= 5 {
		shift := attempt.failures - 5
		if shift > 8 {
			shift = 8
		}
		block = time.Second * time.Duration(1<<shift)
		if block > 5*time.Minute {
			block = 5 * time.Minute
		}
		attempt.blockedUntil = now.Add(block)
	}
	l.attempts[key] = attempt
	l.cleanupLocked(now)
	return block
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func (l *loginLimiter) cleanupLocked(now time.Time) {
	if len(l.attempts) < maxLoginEntries {
		return
	}
	for key, attempt := range l.attempts {
		if now.Sub(attempt.lastFailure) > loginAttemptTTL {
			delete(l.attempts, key)
		}
	}
	if len(l.attempts) >= maxLoginEntries {
		l.attempts = make(map[string]loginAttempt)
	}
}
