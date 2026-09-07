//go:build g01_live

package main

import (
	"context"
	"io"
)

func readCredentialInput(ctx context.Context, input io.ReadCloser) ([]byte, error) {
	return io.ReadAll(io.LimitReader(input, 16385))
}
