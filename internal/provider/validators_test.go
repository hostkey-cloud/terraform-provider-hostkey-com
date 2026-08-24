package provider

import (
	"strings"
	"testing"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

func TestValidateIPv4(t *testing.T) {
	if err := validateIPv4("192.168.0.1"); err != nil {
		t.Fatalf("valid ipv4: %v", err)
	}
	for _, bad := range []string{"", "not-an-ip", "2001:db8::1"} {
		if err := validateIPv4(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestValidateIPv6(t *testing.T) {
	if err := validateIPv6("2001:db8::1"); err != nil {
		t.Fatalf("valid ipv6: %v", err)
	}
	if err := validateIPv6("192.168.0.1"); err == nil {
		t.Fatal("expected error for ipv4 as AAAA")
	}
}

func TestValidateSSHPublicKey(t *testing.T) {
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKaccsshkeyfortestonlydonotuseanywhereelse comment"
	if err := validateSSHPublicKey(pub); err != nil {
		t.Fatalf("valid key: %v", err)
	}
	if err := validateSSHPublicKey("not-a-key"); err == nil {
		t.Fatal("expected error for garbage key")
	}
}

func TestValidateInvapiBaseURL(t *testing.T) {
	own := strings.TrimSuffix(invapi.DefaultBaseURL, "/")
	for _, u := range []string{"", own, "http://127.0.0.1:9/"} {
		if err := validateInvapiBaseURL(u); err != nil {
			t.Fatalf("%q: %v", u, err)
		}
	}
	sibHTTP := "http://" + invapi.SiblingAPIHostHint + "/"
	sibHTTPS := "https://" + invapi.SiblingAPIHostHint + "/"
	for _, bad := range []string{"ftp://x", "not-a-url", "https://", sibHTTP, sibHTTPS} {
		if err := validateInvapiBaseURL(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestLocationCodePattern(t *testing.T) {
	for _, loc := range []string{"NL", "US", "RU", "de"} {
		if !locationCodeRe.MatchString(loc) {
			t.Fatalf("%q should match", loc)
		}
	}
	if locationCodeRe.MatchString("TOOLONG") {
		t.Fatal("expected reject long code")
	}
}
