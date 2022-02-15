package handler

import (
	"io"
	"net/http/httptest"
)

type ctxKeyType struct{}

type fakeCloseReadCloser struct {
	io.ReadCloser
}

type mockHTTPRespWriter struct {
	*httptest.ResponseRecorder
}
