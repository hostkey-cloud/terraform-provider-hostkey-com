package invapi

import "testing"

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
