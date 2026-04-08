package middleware

import (
	"net/http"
)

// killChecker is satisfied by *redisc.Client and any test stub.
type killChecker interface{ IsKilled() bool }

// KillSwitch blocks all requests when the Redis kill switch is activated.
// Operators publish "kill:all" to vela:killswitch to trigger a 503 response.
func KillSwitch(rc killChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rc.IsKilled() {
				http.Error(w, "service temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
