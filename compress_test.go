package compress

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goroute/route"
	"github.com/stretchr/testify/assert"
)

func TestGzip(t *testing.T) {
	mux := route.NewServeMux()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := mux.NewContext(req, rec)

	// Skip if no Accept-Encoding header
	h := New()(func(c route.Context) error {
		c.Response().Write([]byte("test")) // For Content-Type sniffing
		return nil
	})
	h(c)

	assert := assert.New(t)

	assert.Equal("test", rec.Body.String())

	// Gzip
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(route.HeaderAcceptEncoding, gzipScheme)
	rec = httptest.NewRecorder()
	c = mux.NewContext(req, rec)
	h(c)
	assert.Equal(gzipScheme, rec.Header().Get(route.HeaderContentEncoding))
	assert.Contains(rec.Header().Get(route.HeaderContentType), route.MIMETextPlain)
	r, err := gzip.NewReader(rec.Body)
	if assert.NoError(err) {
		buf := new(bytes.Buffer)
		defer r.Close()
		buf.ReadFrom(r)
		assert.Equal("test", buf.String())
	}

	// Gzip chunked
	chunkBuf := make([]byte, 5)
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(route.HeaderAcceptEncoding, gzipScheme)
	rec = httptest.NewRecorder()

	c = mux.NewContext(req, rec)
	_ = New()(func(c route.Context) error {
		c.Response().Header().Set("Content-Type", "text/event-stream")
		c.Response().Header().Set("Transfer-Encoding", "chunked")

		// Write and flush the first part of the data
		c.Response().Write([]byte("test\n"))
		c.Response().Flush()

		// Read the first part of the data
		assert.True(rec.Flushed)
		assert.Equal(gzipScheme, rec.Header().Get(route.HeaderContentEncoding))
		r.Reset(rec.Body)

		_, err = io.ReadFull(r, chunkBuf)
		assert.NoError(err)
		assert.Equal("test\n", string(chunkBuf))

		// Write and flush the second part of the data
		c.Response().Write([]byte("test\n"))
		c.Response().Flush()

		_, err = io.ReadFull(r, chunkBuf)
		assert.NoError(err)
		assert.Equal("test\n", string(chunkBuf))

		// Write the final part of the data and return
		c.Response().Write([]byte("test"))
		return nil
	})(c)

	buf := new(bytes.Buffer)
	defer r.Close()
	buf.ReadFrom(r)
	assert.Equal("test", buf.String())
}

func TestGzipNoContent(t *testing.T) {
	mux := route.NewServeMux()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(route.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	c := mux.NewContext(req, rec)
	h := New()(func(c route.Context) error {
		return c.NoContent(http.StatusNoContent)
	})
	if assert.NoError(t, h(c)) {
		assert.Empty(t, rec.Header().Get(route.HeaderContentEncoding))
		assert.Empty(t, rec.Header().Get(route.HeaderContentType))
		assert.Equal(t, 0, len(rec.Body.Bytes()))
	}
}

func TestGzipErrorReturned(t *testing.T) {
	mux := route.NewServeMux()
	mux.Use(New())
	mux.GET("/", func(c route.Context) error {
		return route.ErrNotFound
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(route.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, rec.Header().Get(route.HeaderContentEncoding))
}

func TestGzipWithStatic(t *testing.T) {
	mux := route.NewServeMux()
	mux.Use(New())
	mux.Static("/test", "testdata/images")
	req := httptest.NewRequest(http.MethodGet, "/test/walle.png", nil)
	req.Header.Set(route.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	// Data is written out in chunks when Content-Length == "", so only
	// validate the content length if it's not set.
	if cl := rec.Header().Get("Content-Length"); cl != "" {
		assert.Equal(t, cl, rec.Body.Len())
	}
	r, err := gzip.NewReader(rec.Body)
	if assert.NoError(t, err) {
		defer r.Close()
		want, err := ioutil.ReadFile("testdata/images/walle.png")
		if assert.NoError(t, err) {
			buf := new(bytes.Buffer)
			buf.ReadFrom(r)
			assert.Equal(t, want, buf.Bytes())
		}
	}
}
