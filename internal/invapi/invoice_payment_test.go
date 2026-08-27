package invapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOrderAwaitsPayment(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"Paid", false},
		{"Pending", false},
		{"Active", false},
		{"Unpaid", true},
		{"unpaid", true},
		{"Not Paid", true},
		{"Awaiting Payment", true},
		{"waiting for payment", true},
		{"Payment Pending", true},
	}
	for _, tc := range cases {
		if got := OrderAwaitsPayment(tc.in); got != tc.want {
			t.Fatalf("OrderAwaitsPayment(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestDecodeAPIError_SuccessResult(t *testing.T) {
	t.Parallel()
	if err := decodeAPIError([]byte(`{"result":"success","invoices":{"invoice":[]}}`)); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestPendingPaymentError(t *testing.T) {
	t.Parallel()
	err := &PendingPaymentError{Invoice: 42, Status: "Unpaid"}
	if !IsPendingPayment(err) {
		t.Fatal("expected IsPendingPayment")
	}
	if IsPendingTerminal(err) {
		t.Fatal("payment wait must not be terminal")
	}
	if got := err.Error(); got != `WHMCS invoice 42 is unpaid (status "Unpaid")` {
		t.Fatalf("Error()=%q", got)
	}
}

func TestWaitForInvoicePayment(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case strings.Contains(r.URL.Path, "auth"):
			_, _ = io.WriteString(w, `{"token":"tok","token_expire":9999999999}`)
		case r.Form.Get("action") == "get_invoices":
			n := hits.Add(1)
			st := "Unpaid"
			if n >= 3 {
				st = "Paid"
			}
			_, _ = fmt.Fprintf(w, `{"result":"success","invoices":{"invoice":[{"id":99,"status":%q}]}}`, st)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL + "/"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	auth := NewTokenManager("key", 3600, client)
	client.SetAuth(auth)

	ctx := context.Background()
	if err := client.WaitForInvoicePayment(ctx, 99, WaitOptions{
		PollInterval: 5 * time.Millisecond,
		Timeout:      2 * time.Second,
	}); err != nil {
		t.Fatalf("WaitForInvoicePayment: %v", err)
	}
	if hits.Load() < 3 {
		t.Fatalf("hits=%d want >=3", hits.Load())
	}
}

func TestWaitForInvoicePayment_Timeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if strings.Contains(r.URL.Path, "auth") {
			_, _ = io.WriteString(w, `{"token":"tok","token_expire":9999999999}`)
			return
		}
		_, _ = io.WriteString(w, `{"result":"success","invoices":{"invoice":[{"id":7,"status":"Unpaid"}]}}`)
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL + "/"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	client.SetAuth(NewTokenManager("key", 3600, client))

	err = client.WaitForInvoicePayment(context.Background(), 7, WaitOptions{
		PollInterval: 5 * time.Millisecond,
		Timeout:      40 * time.Millisecond,
	})
	if !IsPendingPayment(err) {
		t.Fatalf("want PendingPaymentError, got %v", err)
	}
}
