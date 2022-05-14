package httputil

import "net/http"

// CopyHeaders copies http headers from source to destination, it
// does not override, but adds multiple headers.
func CopyHeaders(dst http.Header, src http.Header) {
	for k, vv := range src {
		dst[k] = append(dst[k], vv...)
	}
}
