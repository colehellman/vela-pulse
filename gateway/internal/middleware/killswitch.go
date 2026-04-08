package middleware

import (
	"net/http"
)

// KillChecker is satisfied by *redisc.Client and any test stub.
type KillChecker interface{ IsKilled() bool }

// KillSwitch blocks all requests when the Redis kill switch is activated.
// Operators use PUBLISH vela:killswitch kill:all to activate (503) and
// PUBLISH vela:killswitch resume:all to deactivate. See redis.SubscribeKillSwitch.
func KillSwitch(rc KillChecker) func(http.Handler) http.Handler {
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
