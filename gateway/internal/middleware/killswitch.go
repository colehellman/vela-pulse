package middleware

import (
	"net/http"

	redisc "github.com/colehellman/vela-pulse/gateway/internal/redis"
)

// KillSwitch blocks all requests when the Redis kill switch is activated.
// Operators publish "kill:all" to vela:killswitch to trigger a 503 response.
func KillSwitch(rc *redisc.Client) func(http.Handler) http.Handler {
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
