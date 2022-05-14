// Package httputil a package for handling http data tasks
package httputil

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/packruler/pretty-error/compressutil"
	"github.com/packruler/pretty-error/types"
)

// CodeCatcher a CodeCatcher used to simplify ResponseWriter data and manipulation.
type CodeCatcher struct {
	buffer             bytes.Buffer
	lastModified       bool
	wroteHeader        bool
	headerMap          http.Header
	code               int
	httpCodeRanges     types.HTTPCodeRanges
	caughtFilteredCode bool
	headersSent        bool

	http.ResponseWriter
}

// CodeCatcherWithCloseNotify an extending struct that includes CloseNotify support.
type CodeCatcherWithCloseNotify struct {
	CodeCatcher
}

// ResponseInterceptor interface for providing functionality to external packages.
type ResponseInterceptor interface {
	http.ResponseWriter
	http.Flusher
	GetCode() int
	IsFilteredCode() bool
	GetContent() ([]byte, error)
	GetBuffer() *bytes.Buffer
	SetContent(data []byte)
}

// CloseNotify returns a channel that receives at most a
// single value (true) when the client connection has gone away.
func (codeCatcher *CodeCatcherWithCloseNotify) CloseNotify() <-chan bool {
	if w, ok := codeCatcher.ResponseWriter.(http.CloseNotifier); ok {
		return w.CloseNotify()
	}

	return make(<-chan bool)
}

// NewCodeCatcher create a new instance of codeCatcher or codeCatcherWithCloseNotify based on provided content.
func NewCodeCatcher(responseWriter http.ResponseWriter, httpCodeRanges types.HTTPCodeRanges) ResponseInterceptor {
	catcher := CodeCatcher{
		headerMap:      make(http.Header),
		code:           http.StatusOK, // If backend does not call WriteHeader on us, we consider it's a 200.
		ResponseWriter: responseWriter,
		httpCodeRanges: httpCodeRanges,
	}

	if _, ok := responseWriter.(http.CloseNotifier); ok {
		return &CodeCatcherWithCloseNotify{catcher}
	}

	return &catcher
}

// // WriteHeader into wrapped ResponseWriter.
// func (codeCatcher *codeCatcher) WriteHeader(statusCode int) {
// 	if !codeCatcher.lastModified {
// 		codeCatcher.ResponseWriter.Header().Del("Last-Modified")
// 	}

// 	codeCatcher.wroteHeader = true

// 	// Delegates the Content-Length Header creation to the final body write.
// 	codeCatcher.ResponseWriter.Header().Del("Content-Length")

// 	codeCatcher.ResponseWriter.WriteHeader(statusCode)
// }

// // Write data to internal buffer and mark the status code as http.StatusOK.
// func (codeCatcher *codeCatcher) Write(data []byte) (int, error) {
// 	if !codeCatcher.wroteHeader {
// 		codeCatcher.WriteHeader(http.StatusOK)
// 	}

// 	return codeCatcher.buffer.Write(data)
// }

// GetBuffer get a pointer to the ResponseWriter buffer.
func (codeCatcher *CodeCatcher) GetBuffer() *bytes.Buffer {
	return &codeCatcher.buffer
}

// GetContent load the content currently in the internal buffer
// acodeCatcherounting for possible encoding.
func (codeCatcher *CodeCatcher) GetContent() ([]byte, error) {
	encoding := codeCatcher.getContentEncoding()

	return compressutil.Decode(codeCatcher.GetBuffer(), encoding)
}

// SetContent write data to the internal ResponseWriter buffer
// and match initial encoding.
func (codeCatcher *CodeCatcher) SetContent(data []byte) {
	encoding := codeCatcher.getContentEncoding()

	bodyBytes, _ := compressutil.Encode(data, encoding)

	if !codeCatcher.wroteHeader {
		codeCatcher.WriteHeader(http.StatusOK)
	}

	if _, err := codeCatcher.ResponseWriter.Write(bodyBytes); err != nil {
		log.Printf("unable to write rewriten body: %v", err)
		codeCatcher.LogHeaders()
	}
}

// SupportsProcessing determine if http.Request is supported by this plugin.
func SupportsProcessing(request *http.Request) bool {
	// Ignore non GET requests
	if request.Method != "GET" {
		return false
	}

	if strings.Contains(request.Header.Get("Upgrade"), "websocket") {
		// log.Printf("Ignoring websocket request for %s", request.RequestURI)
		return false
	}

	return true
}

func (codeCatcher *CodeCatcher) getHeader(headerName string) string {
	return codeCatcher.ResponseWriter.Header().Get(headerName)
}

// LogHeaders writes current response headers.
func (codeCatcher *CodeCatcher) LogHeaders() {
	log.Printf("Error Headers: %v", codeCatcher.ResponseWriter.Header())
}

// getContentEncoding get the Content-Encoding header value.
func (codeCatcher *CodeCatcher) getContentEncoding() string {
	return codeCatcher.getHeader("Content-Encoding")
}

// getContentType get the Content-Encoding header value.
func (codeCatcher *CodeCatcher) getContentType() string {
	return codeCatcher.getHeader("Content-Type")
}

func (codeCatcher *CodeCatcher) getSetCookie() string {
	return codeCatcher.getHeader("Set-Cookie")
}

// SupportsWriting determine if response headers support updating content.
func (codeCatcher *CodeCatcher) SupportsWriting() bool {
	setCookie := codeCatcher.getSetCookie()

	return !strings.Contains(setCookie, "XSRF-TOKEN")
}

// SupportsProcessing determine if HttpWrapper is supported by this plugin based on encoding.
func (codeCatcher *CodeCatcher) SupportsProcessing() bool {
	contentType := codeCatcher.getContentType()

	// If content type does not match return values with false
	if contentType != "" && !strings.Contains(contentType, "text") {
		return false
	}

	encoding := codeCatcher.getContentEncoding()

	// If content type is supported validate encoding as well
	switch encoding {
	case "gzip":
		fallthrough
	case "deflate":
		fallthrough
	case "identity":
		fallthrough
	case "":
		return true
	default:
		return false
	}
}

// SetLastModified update the local lastModified variable from non-package-based users.
func (codeCatcher *CodeCatcher) SetLastModified(value bool) {
	codeCatcher.lastModified = value
}

// // GetStatus get the response status code.
// func (codeCatcher *codeCatcher) GetStatus() int16 {
// 	return codeCatcher.status
// }

// START COPY

// Header get http.Header contained in CodeCatcher.
func (codeCatcher *CodeCatcher) Header() http.Header {
	if codeCatcher.headerMap == nil {
		codeCatcher.headerMap = make(http.Header)
	}

	return codeCatcher.headerMap
}

// GetCode get status code contained in CodeCatcher.
func (codeCatcher *CodeCatcher) GetCode() int {
	return codeCatcher.code
}

// IsFilteredCode returns whether the codeCatcher received a response code among the ones it is watching,
// and for which the response should be deferred to the error handler.
func (codeCatcher *CodeCatcher) IsFilteredCode() bool {
	return codeCatcher.caughtFilteredCode
}

func (codeCatcher *CodeCatcher) Write(buf []byte) (int, error) {
	// If WriteHeader was already called from the caller, this is a NOOP.
	// Otherwise, codeCatcher.code is actually a 200 here.
	codeCatcher.WriteHeader(codeCatcher.code)

	// if codeCatcher.caughtFilteredCode {
	// 	// We don't care about the contents of the response,
	// 	// since we want to serve the ones from the error page,
	// 	// so we just drop them.
	// 	return len(buf), nil
	// }

	return codeCatcher.ResponseWriter.Write(buf)
}

// WriteHeader status code to CodeCatcher.
func (codeCatcher *CodeCatcher) WriteHeader(code int) {
	if codeCatcher.headersSent || codeCatcher.caughtFilteredCode {
		return
	}

	codeCatcher.code = code
	for _, block := range codeCatcher.httpCodeRanges {
		if codeCatcher.code >= block[0] && codeCatcher.code <= block[1] {
			codeCatcher.caughtFilteredCode = true
			// it will be up to the caller to send the headers,
			// so it is out of our hands now.
			return
		}
	}

	CopyHeaders(codeCatcher.ResponseWriter.Header(), codeCatcher.Header())
	codeCatcher.ResponseWriter.WriteHeader(codeCatcher.code)
	codeCatcher.headersSent = true
}

// Hijack hijacks the connection.
func (codeCatcher *CodeCatcher) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := codeCatcher.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}

	return nil, nil, fmt.Errorf("%T is not a http.Hijacker", codeCatcher.ResponseWriter)
}

// Flush sends any buffered data to the client.
func (codeCatcher *CodeCatcher) Flush() {
	// If WriteHeader was already called from the caller, this is a NOOP.
	// Otherwise, codeCatcher.code is actually a 200 here.
	codeCatcher.WriteHeader(codeCatcher.code)

	if flusher, ok := codeCatcher.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
