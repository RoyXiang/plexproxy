package handler

import (
	"context"
	"net/http"

	"github.com/urfave/negroni"
)

func RefreshMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lrw := negroni.NewResponseWriter(w)
		next.ServeHTTP(lrw, r)

		if redisClient != nil && lrw.Status() < http.StatusMultipleChoices {
			go func() {
				mu.Lock()
				defer mu.Unlock()

				cmd := redisClient.Eval(context.Background(), redisScriptRemoveAllWithPrefix, []string{}, cachePrefixDynamic)
				_ = cmd.Err()
			}()
		}
	})
}
