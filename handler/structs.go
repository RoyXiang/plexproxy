package handler

import (
	"io"
)

type ctxKeyType struct{}

type fakeCloseReadCloser struct {
	io.ReadCloser
}
