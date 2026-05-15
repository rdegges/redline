package httpx

import (
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_AppliesUserAgent(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cl := NewClient(time.Second, 100, "redline-test")
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := cl.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	DrainAndClose(resp)
	if captured != "redline-test" {
		t.Fatalf("UA = %q", captured)
	}
}

func TestClient_DecodesGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/html")
		gw := gzip.NewWriter(w)
		_, _ = gw.Write([]byte("<html><body>hello</body></html>"))
		_ = gw.Close()
	}))
	defer srv.Close()
	cl := NewClient(time.Second, 100, "ua")
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := cl.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	body, trunc, err := cl.ReadBody(resp)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if trunc {
		t.Fatal("unexpected truncation")
	}
	if string(body) != "<html><body>hello</body></html>" {
		t.Fatalf("body = %q", body)
	}
}

func TestClient_TruncatesLargeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		for i := range buf {
			buf[i] = 'x'
		}
		for i := 0; i < 11; i++ {
			_, _ = w.Write(buf)
		}
	}))
	defer srv.Close()
	cl := NewClient(time.Second, 100, "ua")
	cl.MaxBody = 5 * 1024
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, _ := cl.Do(context.Background(), req)
	body, trunc, err := cl.ReadBody(resp)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !trunc {
		t.Fatal("expected truncation")
	}
	if int64(len(body)) != cl.MaxBody {
		t.Fatalf("body len = %d, want %d", len(body), cl.MaxBody)
	}
}
