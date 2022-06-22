package lrucache

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestHandlerBasics(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test1", nil)
	rw := httptest.NewRecorder()
	shouldBeCalled := true

	handler := NewHttpHandler(1000, time.Second, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Write([]byte("Hello World!"))

		if !shouldBeCalled {
			t.Fatal("fetcher expected to be called")
		}
	}))

	handler.ServeHTTP(rw, r)

	if rw.Code != 200 {
		t.Fatal("unexpected status code")
	}

	if !bytes.Equal(rw.Body.Bytes(), []byte("Hello World!")) {
		t.Fatal("unexpected body")
	}

	rw = httptest.NewRecorder()
	shouldBeCalled = false
	handler.ServeHTTP(rw, r)

	if rw.Code != 200 {
		t.Fatal("unexpected status code")
	}

	if !bytes.Equal(rw.Body.Bytes(), []byte("Hello World!")) {
		t.Fatal("unexpected body")
	}
}

func TestHandlerExpiration(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test1", nil)
	rw := httptest.NewRecorder()
	i := 1
	now := time.Now()

	handler := NewHttpHandler(1000, 1*time.Second, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Expires", now.Add(10*time.Millisecond).Format(http.TimeFormat))
		rw.Write([]byte(strconv.Itoa(i)))
	}))

	handler.ServeHTTP(rw, r)
	if !(rw.Body.String() == strconv.Itoa(1)) {
		t.Fatal("unexpected body")
	}

	i += 1

	time.Sleep(11 * time.Millisecond)
	rw = httptest.NewRecorder()
	handler.ServeHTTP(rw, r)
	if !(rw.Body.String() == strconv.Itoa(1)) {
		t.Fatal("unexpected body")
	}
}
