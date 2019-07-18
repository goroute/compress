package compress

import (
	"bufio"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/goroute/route"
)

// Options defines the config for Gzip middleware.
type Options struct {
	// Skipper defines a function to skip middleware.
	Skipper route.Skipper

	// Gzip compression level.
	// Optional. Default value -1.
	Level int `yaml:"level"`
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

const (
	gzipScheme = "gzip"
)

type Option func(*Options)

func GetDefaultOptions() Options {
	return Options{
		Skipper: route.DefaultSkipper,
		Level:   -1,
	}
}

func Skipper(skipper route.Skipper) Option {
	return func(o *Options) {
		o.Skipper = skipper
	}
}

func Level(level int) Option {
	return func(o *Options) {
		o.Level = level
	}
}

// New return Gzip middleware.
func New(options ...Option) route.MiddlewareFunc {
	// Apply options.
	opts := GetDefaultOptions()
	for _, opt := range options {
		opt(&opts)
	}

	return func(c route.Context, next route.HandlerFunc) error {
		if opts.Skipper(c) {
			return next(c)
		}

		res := c.Response()
		res.Header().Add(route.HeaderVary, route.HeaderAcceptEncoding)
		if strings.Contains(c.Request().Header.Get(route.HeaderAcceptEncoding), gzipScheme) {
			res.Header().Set(route.HeaderContentEncoding, gzipScheme)
			rw := res.Writer
			w, err := gzip.NewWriterLevel(rw, opts.Level)
			if err != nil {
				return err
			}
			defer func() {
				if res.Size == 0 {
					if res.Header().Get(route.HeaderContentEncoding) == gzipScheme {
						res.Header().Del(route.HeaderContentEncoding)
					}
					// We have to reset response to it's pristine state when
					// nothing is written to body or error is returned.
					res.Writer = rw
					w.Reset(ioutil.Discard)
				}
				w.Close()
			}()
			grw := &gzipResponseWriter{Writer: w, ResponseWriter: rw}
			res.Writer = grw
		}
		return next(c)
	}
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if code == http.StatusNoContent {
		w.ResponseWriter.Header().Del(route.HeaderContentEncoding)
	}
	w.Header().Del(route.HeaderContentLength)
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if w.Header().Get(route.HeaderContentType) == "" {
		w.Header().Set(route.HeaderContentType, http.DetectContentType(b))
	}
	return w.Writer.Write(b)
}

func (w *gzipResponseWriter) Flush() {
	w.Writer.(*gzip.Writer).Flush()
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}
