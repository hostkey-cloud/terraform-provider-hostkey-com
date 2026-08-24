package provider

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

const (
	minPollIntervalSecs = 5
	maxPollIntervalSecs = 300

	// root_size is a PERCENTAGE of total disk space per eq/order_instance
	// (default 100), not a GB value.
	minRootSizePercent = 1
	maxRootSizePercent = 100

	minIPv4Amount = 1
	maxIPv4Amount = 64

	minDNSTTL      = 60
	maxDNSTTL      = 2147483647
	maxDNSPriority = 65535

	maxHostnameLen   = 253
	maxSSHKeyNameLen = 128
	maxTagKeyLen     = 64
	maxTagValueLen   = 256

	// Install-time fields forwarded to InvAPI as-is for bare-metal reinstall.
	// These caps avoid unbounded client-side payload sizes (DoS / accidental huge scripts),
	// while staying generous enough for real-world options strings.
	maxOSTemplateLen        = 1024
	maxDeployOptionsLen     = 8192
	maxPostInstallScriptLen = 32768
)

var (
	locationCodeRe = regexp.MustCompile(`^[A-Za-z]{2,4}$`)
	hostnameRe     = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)
	dnsLabelRe     = regexp.MustCompile(`^(@|[a-zA-Z0-9_]([a-zA-Z0-9\-_]{0,61}[a-zA-Z0-9_])?)$`)
	dnsZoneRe      = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)+$`)
	networkPortRe  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.\-]{0,31}$`)
	sshKeyPrefixRe = regexp.MustCompile(`^(ssh-rsa|ssh-ed25519|ecdsa-sha2-[a-z0-9]+|sk-ssh-ed25519@openssh\.com|sk-ecdsa-sha2-nistp256@openssh\.com)\s+`)
)

var dnsRecordTypes = []string{
	"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "PTR", "CAA", "TLSA", "SSHFP",
}

func int64AtLeast(name string, min int64) validator.Int64 {
	return int64RangeValidator{name: name, min: min, max: 0, minOnly: true}
}

func int64Between(name string, min, max int64) validator.Int64 {
	return int64RangeValidator{name: name, min: min, max: max}
}

type int64RangeValidator struct {
	name    string
	min     int64
	max     int64
	minOnly bool
}

func (v int64RangeValidator) Description(_ context.Context) string {
	if v.minOnly {
		return fmt.Sprintf("%s must be >= %d", v.name, v.min)
	}
	return fmt.Sprintf("%s must be between %d and %d", v.name, v.min, v.max)
}

func (v int64RangeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v int64RangeValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	n := req.ConfigValue.ValueInt64()
	if v.minOnly {
		if n < v.min {
			resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("Invalid %s", v.name),
				fmt.Sprintf("%s must be >= %d; got %d", v.name, v.min, n))
		}
		return
	}
	if n < v.min || n > v.max {
		resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("Invalid %s", v.name),
			fmt.Sprintf("%s must be between %d and %d; got %d", v.name, v.min, v.max, n))
	}
}

func stringMaxLen(name string, max int) validator.String {
	return stringLengthValidator{name: name, max: max}
}

type stringLengthValidator struct {
	name string
	max  int
}

func (v stringLengthValidator) Description(_ context.Context) string {
	return fmt.Sprintf("%s must be at most %d characters", v.name, v.max)
}

func (v stringLengthValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringLengthValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := req.ConfigValue.ValueString()
	if len(s) > v.max {
		resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("Invalid %s", v.name),
			fmt.Sprintf("%s must be at most %d characters; got %d", v.name, v.max, len(s)))
	}
}

func locationCodeValidator() validator.String {
	return stringPatternValidator{
		name:      "location",
		pattern:   locationCodeRe,
		desc:      "2-4 letter InvAPI location code (e.g. NL, US, DE)",
		normalize: func(s string) string { return strings.ToUpper(strings.TrimSpace(s)) },
	}
}

func hostnameValidator() validator.String {
	return stringPatternValidator{
		name:    "hostname",
		pattern: hostnameRe,
		desc:    "DNS hostname label (letters, digits, hyphen; optional dots)",
	}
}

func ipv4AddressValidator() validator.String {
	return stringFuncValidator{name: "ip", fn: validateIPv4}
}

func invapiBaseURLValidator() validator.String {
	return stringFuncValidator{name: "base_url", fn: validateInvapiBaseURL}
}

func sshPublicKeyValidator() validator.String {
	return stringFuncValidator{name: "key", fn: validateSSHPublicKey}
}

func dnsZoneValidator() validator.String {
	return stringPatternValidator{
		name:    "name",
		pattern: dnsZoneRe,
		desc:    "FQDN zone name (e.g. example.com)",
	}
}

func dnsRecordNameValidator() validator.String {
	return stringPatternValidator{
		name:    "name",
		pattern: dnsLabelRe,
		desc:    "DNS record name relative to zone (@ or label)",
	}
}

func dnsRecordTypeValidator() validator.String {
	return oneOfStringsCaseInsensitive("type", dnsRecordTypes...)
}

func networkPortValidator() validator.String {
	return stringPatternValidator{
		name:    "port",
		pattern: networkPortRe,
		desc:    "network interface name (e.g. eth0)",
	}
}

type stringPatternValidator struct {
	name      string
	pattern   *regexp.Regexp
	desc      string
	normalize func(string) string
}

func (v stringPatternValidator) Description(_ context.Context) string {
	return fmt.Sprintf("%s must match %s", v.name, v.desc)
}

func (v stringPatternValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringPatternValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := req.ConfigValue.ValueString()
	if v.normalize != nil {
		s = v.normalize(s)
	}
	if !v.pattern.MatchString(s) {
		resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("Invalid %s", v.name),
			fmt.Sprintf("%s must match %s; got %q", v.name, v.desc, req.ConfigValue.ValueString()))
	}
}

type stringFuncValidator struct {
	name string
	fn   func(string) error
}

func (v stringFuncValidator) Description(_ context.Context) string {
	return fmt.Sprintf("%s must be valid", v.name)
}

func (v stringFuncValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringFuncValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if err := v.fn(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("Invalid %s", v.name), err.Error())
	}
}

func oneOfStringsCaseInsensitive(name string, allowed ...string) validator.String {
	set := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		set[strings.ToUpper(a)] = struct{}{}
	}
	return stringOneOfCaseInsensitive{name: name, allowed: allowed, set: set}
}

type stringOneOfCaseInsensitive struct {
	name    string
	allowed []string
	set     map[string]struct{}
}

func (v stringOneOfCaseInsensitive) Description(_ context.Context) string {
	return fmt.Sprintf("%s must be one of: %s", v.name, strings.Join(v.allowed, ", "))
}

func (v stringOneOfCaseInsensitive) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringOneOfCaseInsensitive) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := strings.ToUpper(strings.TrimSpace(req.ConfigValue.ValueString()))
	if _, ok := v.set[s]; ok {
		return
	}
	resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("Invalid %s", v.name),
		fmt.Sprintf("%s must be one of: %s; got %q", v.name, strings.Join(v.allowed, ", "), req.ConfigValue.ValueString()))
}

func validateIPv4(s string) error {
	s = strings.TrimSpace(s)
	if ip := net.ParseIP(s); ip == nil || ip.To4() == nil {
		return fmt.Errorf("must be a valid IPv4 address; got %q", s)
	}
	return nil
}

func validateIPv6(s string) error {
	s = strings.TrimSpace(s)
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() != nil {
		return fmt.Errorf("must be a valid IPv6 address; got %q", s)
	}
	return nil
}

func validateInvapiBaseURL(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return invapi.ValidateConfiguredBaseURL(s)
}

func validateSSHPublicKey(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("must not be empty")
	}
	if !sshKeyPrefixRe.MatchString(s) {
		return fmt.Errorf("must look like an OpenSSH public key (ssh-rsa, ssh-ed25519, ecdsa-sha2-*, …)")
	}
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return fmt.Errorf("public key material is too short")
	}
	return nil
}

func sshPublicKeyMaterial(s string) string {
	parts := strings.Fields(strings.TrimSpace(s))
	if len(parts) < 2 {
		return strings.TrimSpace(s)
	}
	return parts[0] + " " + parts[1]
}

func sshPublicKeysEquivalent(a, b string) bool {
	return sshPublicKeyMaterial(a) == sshPublicKeyMaterial(b)
}

func invapiServerIDValidator() validator.Int64 {
	return int64FuncValidator{name: "server_id", fn: func(n int64) error {
		if n < 1 {
			return fmt.Errorf("server_id must be >= 1; got %d", n)
		}
		return nil
	}}
}

type int64FuncValidator struct {
	name string
	fn   func(int64) error
}

func (v int64FuncValidator) Description(_ context.Context) string {
	return fmt.Sprintf("%s must be a valid InvAPI server id", v.name)
}

func (v int64FuncValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v int64FuncValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if err := v.fn(req.ConfigValue.ValueInt64()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("Invalid %s", v.name), err.Error())
	}
}
