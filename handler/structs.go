package handler

import (
	"net/http/httptest"
)

type ctxKeyType struct{}

type mockHTTPRespWriter struct {
	*httptest.ResponseRecorder
}
