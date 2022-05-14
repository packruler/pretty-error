// Package pretty_error a plugin to rewrite response body.
package pretty_error

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"

	"github.com/packruler/pretty-error/httputil"
	"github.com/packruler/pretty-error/types"
)

// Rewrite holds one rewrite body configuration.
type Rewrite struct {
	Regex       string `json:"regex,omitempty"`
	Replacement string `json:"replacement,omitempty"`
}

// Config holds the plugin configuration.
type Config struct {
	LastModified bool      `json:"lastModified,omitempty"`
	Rewrites     []Rewrite `json:"rewrites,omitempty"`
	Status       []string  `json:"status,omitempty" toml:"status,omitempty" yaml:"status,omitempty" export:"true"`
}

// CreateConfig creates and initializes the plugin configuration.
func CreateConfig() *Config {
	return &Config{}
}

type rewrite struct {
	regex       *regexp.Regexp
	replacement []byte
}

type rewriteBody struct {
	name           string
	next           http.Handler
	rewrites       []rewrite
	lastModified   bool
	httpCodeRanges types.HTTPCodeRanges
}

type codeCatcherWithCloseNotify struct {
	*codeCatcher
}

type responseInterceptor interface {
	http.ResponseWriter
	http.Flusher
	getCode() int
	isFilteredCode() bool
}

// codeCatcher is a response writer that detects as soon as possible whether the
// response is a code within the ranges of codes it watches for. If it is, it
// simply drops the data from the response. Otherwise, it forwards it directly to
// the original client (its responseWriter) without any buffering.
type codeCatcher struct {
	headerMap          http.Header
	code               int
	httpCodeRanges     types.HTTPCodeRanges
	caughtFilteredCode bool
	responseWriter     http.ResponseWriter
	headersSent        bool
}

// New creates and returns a new rewrite body plugin instance.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	httpCodeRanges, err := types.NewHTTPCodeRanges(config.Status)
	if err != nil {
		return nil, err
	}

	rewrites := make([]rewrite, len(config.Rewrites))

	for index, rewriteConfig := range config.Rewrites {
		regex, err := regexp.Compile(rewriteConfig.Regex)
		if err != nil {
			return nil, fmt.Errorf("error compiling regex %q: %w", rewriteConfig.Regex, err)
		}

		rewrites[index] = rewrite{
			regex:       regex,
			replacement: []byte(rewriteConfig.Replacement),
		}
	}

	log.Printf("New: %v", httpCodeRanges)

	return &rewriteBody{
		name:           name,
		next:           next,
		rewrites:       rewrites,
		lastModified:   config.LastModified,
		httpCodeRanges: httpCodeRanges,
	}, nil
}

func (bodyRewrite *rewriteBody) ServeHTTP(response http.ResponseWriter, req *http.Request) {
	// allow default http.ResponseWriter to handle calls targeting WebSocket upgrades and non GET methods
	if !httputil.SupportsProcessing(req) {
		bodyRewrite.next.ServeHTTP(response, req)

		return
	}

	// wrappedWriter := &httputil.ResponseWrapper{
	// 	ResponseWriter: response,
	// }

	log.Print("Before catcher")

	catcher := newCodeCatcher(response, bodyRewrite.httpCodeRanges)
	log.Printf("Catcher: %v", catcher)
	bodyRewrite.next.ServeHTTP(catcher, req)

	log.Print("After serve")

	log.Printf("Status: %d", catcher.getCode())

	if !catcher.isFilteredCode() {
		return
	}

	// look into using https://pkg.go.dev/net/http#RoundTripper
	// bodyRewrite.next.ServeHTTP(wrappedWriter, req)

	// if !wrappedWriter.SupportsProcessing() || !wrappedWriter.SupportsWriting() {
	// 	// We are ignoring these any errors because the content should be unchanged here.
	// 	// This could "error" if writing is not supported but content will return properly.
	// 	_, _ = response.Write(wrappedWriter.GetBuffer().Bytes())

	// 	return
	// }

	// bodyBytes, err := catcher.GetContent()
	// if err != nil {
	// 	log.Printf("Error loading content: %v", err)

	// 	if _, err := response.Write(catcher.GetBuffer().Bytes()); err != nil {
	// 		log.Printf("unable to write error content: %v", err)
	// 	}

	// 	return
	// }

	// // log.Printf("Body: %s", bodyBytes)
	// catcher.SetContent(bodyBytes)
	log.Printf("Status: %d", catcher.getCode())
}

// CloseNotify returns a channel that receives at most a
// single value (true) when the client connection has gone away.
func (cc *codeCatcherWithCloseNotify) CloseNotify() <-chan bool {
	if w, ok := cc.responseWriter.(http.CloseNotifier); ok {
		return w.CloseNotify()
	}

	return make(<-chan bool)
}

func newCodeCatcher(responseWriter http.ResponseWriter, httpCodeRanges types.HTTPCodeRanges) responseInterceptor {
	catcher := &codeCatcher{
		headerMap:      make(http.Header),
		code:           http.StatusOK, // If backend does not call WriteHeader on us, we consider it's a 200.
		responseWriter: responseWriter,
		httpCodeRanges: httpCodeRanges,
	}

	if _, ok := responseWriter.(http.CloseNotifier); ok {
		return &codeCatcherWithCloseNotify{catcher}
	}

	return catcher
}

func (cc *codeCatcher) Header() http.Header {
	if cc.headerMap == nil {
		cc.headerMap = make(http.Header)
	}

	return cc.headerMap
}

func (cc *codeCatcher) getCode() int {
	return cc.code
}

// isFilteredCode returns whether the codeCatcher received a response code among the ones it is watching,
// and for which the response should be deferred to the error handler.
func (cc *codeCatcher) isFilteredCode() bool {
	return cc.caughtFilteredCode
}

func (cc *codeCatcher) Write(buf []byte) (int, error) {
	// If WriteHeader was already called from the caller, this is a NOOP.
	// Otherwise, cc.code is actually a 200 here.
	cc.WriteHeader(cc.code)

	if cc.caughtFilteredCode {
		// We don't care about the contents of the response,
		// since we want to serve the ones from the error page,
		// so we just drop them.
		return len(buf), nil
	}

	return cc.responseWriter.Write(buf)
}

func (cc *codeCatcher) WriteHeader(code int) {
	if cc.headersSent || cc.caughtFilteredCode {
		return
	}

	cc.code = code
	for _, block := range cc.httpCodeRanges {
		if cc.code >= block[0] && cc.code <= block[1] {
			cc.caughtFilteredCode = true
			// it will be up to the caller to send the headers,
			// so it is out of our hands now.
			return
		}
	}

	httputil.CopyHeaders(cc.responseWriter.Header(), cc.Header())
	cc.responseWriter.WriteHeader(cc.code)
	cc.headersSent = true
}

// Hijack hijacks the connection.
func (cc *codeCatcher) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := cc.responseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}

	return nil, nil, fmt.Errorf("%T is not a http.Hijacker", cc.responseWriter)
}

// Flush sends any buffered data to the client.
func (cc *codeCatcher) Flush() {
	// If WriteHeader was already called from the caller, this is a NOOP.
	// Otherwise, cc.code is actually a 200 here.
	cc.WriteHeader(cc.code)

	if flusher, ok := cc.responseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
