package api

// Action rate limits (spec section 10: "Лимиты действий: не более N задач/файлов за период").
//
// Distinct from the login throttle in ratelimit.go, which protects authentication. This guards
// against runaway volume from an authenticated account — a broken script or a careless import
// creating thousands of tasks.
//
// Counting is per user, per action, in hourly buckets (migration 0009). An hourly counter is
// deliberately coarse: it stops runaway automation without the storage and write cost of a full
// audit log, and the exact boundary doesn't matter for a safety limit.
//
// Both limits default to 0, which means unlimited — consistent with every other limit in the
// product, and it means upgrading never suddenly caps an instance that was working fine.

import (
	"context"
	"fmt"
	"net/http"
)

// actionAllowed reports whether the user may perform one more of `action` this hour, and returns
// a message suitable for showing the user when they may not.
//
// Fails open: if the counter can't be read, the action proceeds. A limit is a guardrail, and a
// transient database hiccup shouldn't block legitimate work.
func (a *API) actionAllowed(ctx context.Context, userID int64, action, settingKey string, def int) (bool, string) {
	limit := a.intSetting(ctx, settingKey, def)
	if limit <= 0 {
		return true, "" // 0 = unlimited
	}
	var used int
	if err := a.DB.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(count),0)::int FROM action_counters
		WHERE user_id=$1 AND action=$2 AND bucket_hour > now() - interval '1 hour'`,
		userID, action).Scan(&used); err != nil {
		return true, ""
	}
	if used >= limit {
		return false, fmt.Sprintf("rate limit reached: %d %s per hour. Try again later.", limit, action)
	}
	return true, ""
}

// countAction records one occurrence. Called only after the action succeeded, so a rejected
// request doesn't consume quota.
func (a *API) countAction(ctx context.Context, userID int64, action string) {
	_, _ = a.DB.Pool.Exec(ctx, `
		INSERT INTO action_counters(user_id, action, bucket_hour, count)
		VALUES($1,$2,date_trunc('hour', now()),1)
		ON CONFLICT (user_id, action, bucket_hour) DO UPDATE SET count = action_counters.count + 1`,
		userID, action)
}

// enforceAction is the one-call form used by handlers: it checks the limit and, when exceeded,
// writes the 429 response itself and reports false so the caller just returns.
func (a *API) enforceAction(w http.ResponseWriter, r *http.Request, userID int64, action, settingKey string, def int) bool {
	okToRun, msg := a.actionAllowed(r.Context(), userID, action, settingKey, def)
	if !okToRun {
		errJSON(w, http.StatusTooManyRequests, msg)
		return false
	}
	return true
}
