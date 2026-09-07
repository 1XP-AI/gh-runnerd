package livecanary

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

type countingBody struct {
	remaining, read int64
	closed          bool
}

func (b *countingBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), b.remaining)
	clear(p[:n])
	b.remaining -= n
	b.read += n
	return int(n), nil
}
func (b *countingBody) Close() error { b.closed = true; return nil }

func TestResponseReaderConsumesOnlyBudgetPlusOneAndRejectsTruncation(t *testing.T) {
	for _, size := range []int64{32, responseBodyLimit, responseBodyLimit + 4096} {
		source := &countingBody{remaining: size}
		body := &responseBudgetBody{source: source, remaining: responseBodyLimit}
		data, err := io.ReadAll(body)
		if size <= responseBodyLimit {
			if err != nil || int64(len(data)) != size {
				t.Fatal("in-budget body altered")
			}
		} else if !errors.Is(err, errResponseBudget) || source.read != responseBodyLimit+1 || !source.closed {
			t.Fatal("overflow was truncated or read without a bound")
		}
	}
}

func TestResponseBudgetAppliesAfterGzipDecompression(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		zip := gzip.NewWriter(w)
		_, _ = io.CopyN(zip, strings.NewReader(strings.Repeat("s", int(responseBodyLimit)+1)), responseBodyLimit+1)
		_ = zip.Close()
	}))
	defer server.Close()
	transport := server.Client().Transport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: withResponseBudget(transport)}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal("fixture transport failed")
	}
	defer response.Body.Close()
	_, err = io.ReadAll(response.Body)
	if !errors.Is(err, errResponseBudget) {
		t.Fatal("compressed response bypassed decoded-body budget")
	}
}

func TestSharedTransportRejectsOversizeSuccessAndErrorBodies(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		for _, chunked := range []bool{false, true} {
			t.Run(strconv.Itoa(status)+"/chunked="+strconv.FormatBool(chunked), func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if !chunked {
						w.Header().Set("Content-Length", strconv.FormatInt(responseBodyLimit+1024, 10))
					}
					w.WriteHeader(status)
					if chunked {
						w.(http.Flusher).Flush()
					}
					_, _ = io.CopyN(w, strings.NewReader(strings.Repeat("s", int(responseBodyLimit)+1024)), responseBodyLimit+1024)
				}))
				defer server.Close()
				transport := http.DefaultTransport.(*http.Transport).Clone()
				transport.Proxy = nil
				transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
					if address != server.Listener.Addr().String() {
						t.Fatal("non-fixture request")
					}
					return (&net.Dialer{}).DialContext(ctx, network, address)
				}
				client := &http.Client{Transport: withResponseBudget(transport)}
				response, err := client.Get(server.URL)
				if err == nil {
					defer response.Body.Close()
					_, err = io.ReadAll(response.Body)
				}
				if err == nil || calls != 1 {
					t.Fatal("unbounded response accepted or retried")
				}
			})
		}
	}
}
