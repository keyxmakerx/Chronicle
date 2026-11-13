package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ForceSessionRevalidation makes every live session check itself against the
// database on its next request, instead of waiting out the usual interval.
// It is for the moment the database has been replaced: a session whose user is
// missing from the restored data is then destroyed, and one whose user
// remains picks up the restored admin flag and details. Sessions are kept,
// not deleted, so people whose accounts survive stay signed in. It returns how
// many sessions it marked.
func ForceSessionRevalidation(ctx context.Context, svc AuthService) (int, error) {
	s, ok := svc.(*authService)
	if !ok || s.redis == nil {
		return 0, nil
	}
	marked := 0
	iter := s.redis.Scan(ctx, 0, sessionKeyPrefix+"*", 200).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		data, err := s.redis.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var sess Session
		if json.Unmarshal(data, &sess) != nil {
			// A session that cannot be read cannot be trusted either.
			s.redis.Del(ctx, key)
			continue
		}
		sess.LastValidated = time.Time{}
		out, err := json.Marshal(sess)
		if err != nil {
			continue
		}
		// KeepTTL + XX: never extend a session or revive one that just ended.
		if err := s.redis.SetXX(ctx, key, out, redis.KeepTTL).Err(); err == nil {
			marked++
		}
	}
	if err := iter.Err(); err != nil {
		return marked, fmt.Errorf("scanning sessions: %w", err)
	}
	return marked, nil
}

// SessionCookieNames returns the session cookie names the auth plugin sets,
// so the maintenance gate, which cannot import this package, can be pinned
// to the same names by a test.
func SessionCookieNames() []string {
	return []string{sessionCookieSecureName, sessionCookieName}
}
