//go:build g01_live

package main

import (
	"context"
	"errors"
	"io"
)

func readCredentialInput(ctx context.Context, input io.ReadCloser) ([]byte, error) {
	refused := errors.New("credential input refused")
	if ctx == nil || input == nil {
		return nil, refused
	}
	defer input.Close()
	if ctx.Err() != nil {
		return nil, refused
	}
	// Production input is os.Stdin: closing its owned descriptor interrupts a
	// blocked pipe read. The callback never formats or records credential bytes.
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	defer stop()
	data, err := io.ReadAll(io.LimitReader(input, 16385))
	if err != nil || ctx.Err() != nil || len(data) > 16384 {
		clear(data)
		return nil, refused
	}
	return data, nil
}
