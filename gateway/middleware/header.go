package middleware

import "net/http"

func HeaderMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.Header {
			r.Header.Set("Grpc-Metadata-"+k, v[0])
		}
		next(w, r)
	}
}
