package invapi

import (
	"strings"
	"testing"
)

func TestValidateBaseURL(t *testing.T) {
	ownHTTPS := strings.TrimSuffix(DefaultBaseURL, "/")
	sibHTTPS := "https://" + SiblingAPIHostHint + "/"
	for _, u := range []string{
		ownHTTPS,
		sibHTTPS, // scheme check only — portal filter is ValidateConfiguredBaseURL
		"http://127.0.0.1:8080/",
		"http://localhost/invapi/",
	} {
		if err := ValidateBaseURL(u); err != nil {
			t.Fatalf("%q: %v", u, err)
		}
	}
	ownHTTP := "http://" + strings.TrimPrefix(strings.TrimSuffix(DefaultBaseURL, "/"), "https://") + "/"
	for _, bad := range []string{
		"",
		"ftp://x",
		"not-a-url",
		"https://",
		ownHTTP,
		"http://evil.example/",
	} {
		if err := ValidateBaseURL(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestValidateConfiguredBaseURL(t *testing.T) {
	if err := ValidateConfiguredBaseURL(DefaultBaseURL); err != nil {
		t.Fatalf("own portal: %v", err)
	}
	if err := ValidateConfiguredBaseURL("http://127.0.0.1:8080/"); err != nil {
		t.Fatalf("loopback: %v", err)
	}
	sib := "https://" + SiblingAPIHostHint + "/"
	err := ValidateConfiguredBaseURL(sib)
	if err == nil {
		t.Fatal("expected reject sibling portal")
	}
	if !strings.Contains(err.Error(), SiblingProviderSource) {
		t.Fatalf("sibling error should mention %s: %v", SiblingProviderSource, err)
	}
	if err := ValidateConfiguredBaseURL("https://evil.example/"); err == nil {
		t.Fatal("expected reject unrelated host")
	}
}

func TestAllowedInvAPIRewrite(t *testing.T) {
	cur := DefaultBaseURL
	host := strings.TrimPrefix(strings.TrimSuffix(DefaultBaseURL, "/"), "https://")
	ok, err := CanonicalInvAPIBaseURL(host)
	if err != nil {
		t.Fatal(err)
	}
	if err := allowedInvAPIRewrite(cur, ok); err != nil {
		t.Fatalf("own portal rewrite: %v", err)
	}
	if err := allowedInvAPIRewrite(cur, "https://evil.example/"); err == nil {
		t.Fatal("expected reject attacker host")
	}
	sib, err := CanonicalInvAPIBaseURL(SiblingAPIHostHint)
	if err != nil {
		t.Fatal(err)
	}
	if err := allowedInvAPIRewrite(cur, sib); err == nil {
		t.Fatal("expected reject sibling portal rewrite")
	}
	if err := allowedInvAPIRewrite(cur, "http://"+host+"/"); err == nil {
		t.Fatal("expected reject TLS downgrade")
	}
}

func TestIsHostkeyAPIHost(t *testing.T) {
	host := strings.TrimPrefix(strings.TrimSuffix(DefaultBaseURL, "/"), "https://")
	if !isHostkeyAPIHost(host) || !isHostkeyAPIHost(strings.ToUpper(host)) {
		t.Fatal("expected own portal hosts")
	}
	if isHostkeyAPIHost(SiblingAPIHostHint) {
		t.Fatal("sibling portal must not match isHostkeyAPIHost")
	}
	if isHostkeyAPIHost(PortalDomain+".evil.example") || isHostkeyAPIHost("example.com") {
		t.Fatal("expected reject lookalike")
	}
}

func TestRedactSecrets(t *testing.T) {
	got := redactSecrets(`{"token":"abc123","key":"secret"}`)
	if strings.Contains(got, "abc123") || strings.Contains(got, `"key":"secret"`) {
		t.Fatalf("not redacted: %s", got)
	}
	form := redactSecrets("token=sess&root_pass=Abcdef1%")
	if strings.Contains(form, "sess") || strings.Contains(form, "Abcdef1%") {
		t.Fatalf("form not redacted: %s", form)
	}
}
