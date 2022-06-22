package lrucache

import (
	"bytes"
	"net/http"
	"strconv"
	"time"
)

// HttpHandler is can be used as HTTP Middleware in order to cache requests,
// for example static assets. By default, the request's raw URI is used as key and nothing else.
// Results with a status code other than 200 are cached with a TTL of zero seconds,
// so basically re-fetched as soon as the current fetch is done and a new request
// for that URI is done.
type HttpHandler struct {
	cache      *Cache
	fetcher    http.Handler
	defaultTTL time.Duration

	// Allows overriding the way the cache key is extracted
	// from the http request. The defailt is to use the RequestURI.
	CacheKey func(*http.Request) string
}

var _ http.Handler = (*HttpHandler)(nil)

type cachedResponseWriter struct {
	w          http.ResponseWriter
	statusCode int
	buf        bytes.Buffer
}

type cachedResponse struct {
	headers    http.Header
	statusCode int
	data       []byte
	fetched    time.Time
}

var _ http.ResponseWriter = (*cachedResponseWriter)(nil)

func (crw *cachedResponseWriter) Header() http.Header {
	return crw.w.Header()
}

func (crw *cachedResponseWriter) Write(bytes []byte) (int, error) {
	return crw.buf.Write(bytes)
}

func (crw *cachedResponseWriter) WriteHeader(statusCode int) {
	crw.statusCode = statusCode
}

// Returns a new caching HttpHandler. If no entry in the cache is found or it was too old, `fetcher` is called with
// a modified http.ResponseWriter and the response is stored in the cache. If `fetcher` sets the "Expires" header,
// the ttl is set appropriately (otherwise, the default ttl passed as argument here is used).
// `maxmemory` should be in the unit bytes.
func NewHttpHandler(maxmemory int, ttl time.Duration, fetcher http.Handler) *HttpHandler {
	return &HttpHandler{
		cache:      New(maxmemory),
		defaultTTL: ttl,
		fetcher:    fetcher,
		CacheKey: func(r *http.Request) string {
			return r.RequestURI
		},
	}
}

// gorilla/mux style middleware:
func NewMiddleware(maxmemory int, ttl time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return NewHttpHandler(maxmemory, ttl, next)
	}
}

// Tries to serve a response to r from cache or calls next and stores the response to the cache for the next time.
func (h *HttpHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.ServeHTTP(rw, r)
		return
	}

	cr := h.cache.Get(h.CacheKey(r), func() (interface{}, time.Duration, int) {
		crw := &cachedResponseWriter{
			w:          rw,
			statusCode: 200,
			buf:        bytes.Buffer{},
		}

		h.fetcher.ServeHTTP(crw, r)

		cr := &cachedResponse{
			headers:    rw.Header().Clone(),
			statusCode: crw.statusCode,
			data:       crw.buf.Bytes(),
			fetched:    time.Now(),
		}
		cr.headers.Set("Content-Length", strconv.Itoa(len(cr.data)))

		ttl := h.defaultTTL
		if cr.statusCode != http.StatusOK {
			ttl = 0
		} else if cr.headers.Get("Expires") != "" {
			if expires, err := http.ParseTime(cr.headers.Get("Expires")); err == nil {
				ttl = time.Until(expires)
			}
		}

		return cr, ttl, len(cr.data)
	}).(*cachedResponse)

	for key, val := range cr.headers {
		rw.Header()[key] = val
	}

	cr.headers.Set("Age", strconv.Itoa(int(time.Since(cr.fetched).Seconds())))

	rw.WriteHeader(cr.statusCode)
	rw.Write(cr.data)
}
