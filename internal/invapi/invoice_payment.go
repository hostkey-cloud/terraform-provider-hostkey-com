package invapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PendingPaymentError means the WHMCS invoice for this order is still unpaid
// after WaitForInvoicePayment timed out (or payment was never started). Deploy
// will not start until the customer pays. Soft-exit Create/Update with a
// Warning (keep pending:<invoice>); do not treat as terminal.
type PendingPaymentError struct {
	Invoice int
	Status  string
}

func (e *PendingPaymentError) Error() string {
	if e == nil {
		return "invoice unpaid"
	}
	st := strings.TrimSpace(e.Status)
	if st == "" {
		st = "Unpaid"
	}
	if e.Invoice > 0 {
		return fmt.Sprintf("WHMCS invoice %d is unpaid (status %q)", e.Invoice, st)
	}
	return fmt.Sprintf("invoice is unpaid (status %q)", st)
}

// IsPendingPayment reports whether err means deploy is blocked on invoice payment.
func IsPendingPayment(err error) bool {
	var pe *PendingPaymentError
	return errors.As(err, &pe)
}

// WaitForInvoicePayment polls WHMCS until the invoice is no longer awaiting
// payment (Paid / other non-unpaid status) or Timeout. OnPoll receives the
// latest payment status string (e.g. "Unpaid"). Returns PendingPaymentError if
// still unpaid when the deadline hits. Unknown/empty status is treated as
// "keep waiting" until timeout (InvAPI sometimes lags get_invoices).
func (c *Client) WaitForInvoicePayment(ctx context.Context, invoiceID int, opts WaitOptions) error {
	if invoiceID <= 0 {
		return fmt.Errorf("invoice id required")
	}
	interval := opts.PollInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastStatus := ""
	check := func() error {
		st, err := c.WHMCSInvoicePaymentStatus(ctx, invoiceID)
		if err != nil {
			return err
		}
		lastStatus = st
		if opts.OnPoll != nil {
			hint := st
			if hint == "" {
				hint = "payment status unknown"
			}
			opts.OnPoll(hint)
		}
		if st != "" && !OrderAwaitsPayment(st) {
			return nil
		}
		return ErrPendingNotReady
	}

	if err := check(); err == nil {
		return nil
	} else if err != nil && !errors.Is(err, ErrPendingNotReady) {
		// Transient WHMCS errors: keep polling until timeout.
		lastStatus = err.Error()
		if opts.OnPoll != nil {
			opts.OnPoll(lastStatus)
		}
	}

	for {
		if time.Now().After(deadline) {
			return &PendingPaymentError{Invoice: invoiceID, Status: lastStatus}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if err := check(); err == nil {
			return nil
		} else if err != nil && !errors.Is(err, ErrPendingNotReady) {
			lastStatus = err.Error()
			if opts.OnPoll != nil {
				opts.OnPoll(lastStatus)
			}
		}
	}
}

// OrderAwaitsPayment reports whether order_instance / invoice status means the
// customer must pay before InvAPI starts deploy. Bare "Pending" is deploy-in-
// progress after payment — not a payment wait.
func OrderAwaitsPayment(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	if s == "" {
		return false
	}
	switch s {
	case "unpaid", "not paid", "notpaid", "unpaid invoice":
		return true
	}
	if strings.Contains(s, "awaiting payment") ||
		strings.Contains(s, "waiting for payment") ||
		strings.Contains(s, "waiting payment") ||
		strings.Contains(s, "payment pending") ||
		strings.Contains(s, "pending payment") {
		return true
	}
	return false
}

type whmcsInvoicesListResponse struct {
	Result   string `json:"result"`
	Invoices struct {
		Invoice json.RawMessage `json:"invoice"`
	} `json:"invoices"`
}

type whmcsInvoiceRow struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
}

// WHMCSInvoicePaymentStatus returns the WHMCS payment status for invoiceID
// (e.g. "Paid", "Unpaid") via whmcs/get_invoices. Empty string means unknown.
func (c *Client) WHMCSInvoicePaymentStatus(ctx context.Context, invoiceID int) (string, error) {
	if invoiceID <= 0 {
		return "", fmt.Errorf("invoice id required")
	}
	params := url.Values{}
	params.Set("action", "get_invoices")
	body, err := c.PostForm(ctx, "whmcs", params)
	if err != nil {
		return "", fmt.Errorf("whmcs/get_invoices: %w", err)
	}
	var resp whmcsInvoicesListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("whmcs/get_invoices decode: %w", err)
	}
	rows, err := parseWHMCSInvoiceRows(resp.Invoices.Invoice)
	if err != nil {
		return "", err
	}
	for _, row := range rows {
		if row.ID == invoiceID {
			return strings.TrimSpace(row.Status), nil
		}
	}
	// Fallback: some portals expose payment status on get_invoice.
	return c.whmcsGetInvoiceStatus(ctx, invoiceID)
}

func parseWHMCSInvoiceRows(raw json.RawMessage) ([]whmcsInvoiceRow, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var many []whmcsInvoiceRow
	if err := json.Unmarshal(raw, &many); err == nil {
		return many, nil
	}
	var one whmcsInvoiceRow
	if err := json.Unmarshal(raw, &one); err == nil && one.ID > 0 {
		return []whmcsInvoiceRow{one}, nil
	}
	return nil, fmt.Errorf("whmcs/get_invoices: unexpected invoice payload")
}

func (c *Client) whmcsGetInvoiceStatus(ctx context.Context, invoiceID int) (string, error) {
	params := url.Values{}
	params.Set("action", "get_invoice")
	params.Set("invoice_id", strconv.Itoa(invoiceID))
	body, err := c.PostForm(ctx, "whmcs", params)
	if err != nil {
		return "", fmt.Errorf("whmcs/get_invoice: %w", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", fmt.Errorf("whmcs/get_invoice decode: %w", err)
	}
	for _, key := range []string{"paymentstatus", "payment_status", "invoicestatus", "invoice_status", "status"} {
		if v, ok := envelope[key]; ok {
			if s, ok := v.(string); ok {
				s = strings.TrimSpace(s)
				if s == "" || strings.EqualFold(s, "success") || strings.EqualFold(s, "OK") {
					continue
				}
				return s, nil
			}
		}
	}
	return "", nil
}
