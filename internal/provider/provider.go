package provider

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

const (
	// Environment variable names (not secrets).
	envAPIKey   = "HOSTKEY_API_KEY"   //nolint:gosec // G101: env var name
	envAPIToken = "HOSTKEY_API_TOKEN" //nolint:gosec // G101: env var name
	envBaseURL  = "HOSTKEY_BASE_URL"
	envAPIURL   = "HOSTKEY_API_URL"
)

type hostkeyProvider struct {
	version string
}

type providerModel struct {
	APIKey      types.String `tfsdk:"api_key"`
	BaseURL     types.String `tfsdk:"base_url"`
	TokenTTL    types.Int64  `tfsdk:"token_ttl"`
	HTTPTimeout types.Int64  `tfsdk:"http_timeout"`
	MaxRetries  types.Int64  `tfsdk:"max_retries"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &hostkeyProvider{version: version}
	}
}

func (p *hostkeyProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "hostkey"
	resp.Version = p.version
}

func (p *hostkeyProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Interact with Hostkey (.com) infrastructure via InvAPI (invapi.hostkey.com).",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "Account InvAPI API key (InvAPI -> Username -> API keys). May be set via HOSTKEY_API_KEY or HOSTKEY_API_TOKEN. Docs: " + invapi.DocsAccountKeysURL,
				Optional:    true,
				Sensitive:   true,
			},
			"base_url": schema.StringAttribute{
				Description: "InvAPI base URL override (default https://invapi.hostkey.com/). HTTPS required except localhost. Staging hosts must be on hostkey.com. May be set via HOSTKEY_BASE_URL or HOSTKEY_API_URL. Use hostkey-cloud/hostkey-ru for invapi.hostkey.ru.",
				Optional:    true,
				Validators: []validator.String{
					invapiBaseURLValidator(),
				},
			},
			"token_ttl": schema.Int64Attribute{
				Description: "Session token TTL in seconds for auth/login (default 3600).",
				Optional:    true,
				Validators: []validator.Int64{
					int64Between("token_ttl", 60, 86400),
				},
			},
			"http_timeout": schema.Int64Attribute{
				Description: "HTTP client timeout in seconds for InvAPI requests (default 60).",
				Optional:    true,
				Validators: []validator.Int64{
					int64Between("http_timeout", 5, 600),
				},
			},
			"max_retries": schema.Int64Attribute{
				Description: "Max attempts for retryable InvAPI HTTP failures (default 3).",
				Optional:    true,
				Validators: []validator.Int64{
					int64Between("max_retries", 1, 10),
				},
			},
		},
	}
}

func (p *hostkeyProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := config.APIKey.ValueString()
	if apiKey == "" {
		apiKey = firstEnv(envAPIKey, envAPIToken)
	}
	if apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing API key",
			"Set provider api_key or HOSTKEY_API_KEY / HOSTKEY_API_TOKEN environment variable.",
		)
		return
	}

	baseURL := config.BaseURL.ValueString()
	if baseURL == "" {
		baseURL = firstEnv(envBaseURL, envAPIURL)
	}
	if baseURL == "" {
		baseURL = invapi.DefaultBaseURL
	}
	if err := validateInvapiBaseURL(baseURL); err != nil {
		resp.Diagnostics.AddError("Invalid base_url", err.Error())
		return
	}

	ttl := int(config.TokenTTL.ValueInt64())
	if ttl == 0 {
		ttl = 3600
	}

	httpTimeout := time.Duration(config.HTTPTimeout.ValueInt64()) * time.Second
	if httpTimeout <= 0 {
		httpTimeout = 60 * time.Second
	}

	maxRetries := int(config.MaxRetries.ValueInt64())
	if maxRetries <= 0 {
		maxRetries = 3
	}

	version := p.version
	if version == "" {
		version = "dev"
	}

	client, err := invapi.NewClient(invapi.Config{
		BaseURL:     baseURL,
		HTTPTimeout: httpTimeout,
		MaxRetries:  maxRetries,
		UserAgent:   invapi.ProviderBinaryName + "/" + version,
	}, nil)
	if err != nil {
		resp.Diagnostics.AddError("Client init failed", err.Error())
		return
	}

	auth := invapi.NewTokenManager(apiKey, ttl, client)
	client.SetAuth(auth)

	if _, err := auth.Token(ctx); err != nil {
		if invapi.IsNoAppropriateServers(err) {
			resp.Diagnostics.AddError("InvAPI account has no servers", err.Error())
			return
		}
		resp.Diagnostics.AddError("InvAPI authentication failed", err.Error())
		return
	}

	tflog.Info(ctx, "Configured Hostkey provider", map[string]any{
		"base_url":     client.BaseURL(),
		"http_timeout": httpTimeout.String(),
		"max_retries":  maxRetries,
	})

	resp.DataSourceData = client
	resp.ResourceData = client
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func (p *hostkeyProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewServerResource,
		NewSSHKeyResource,
		NewServerIPResource,
		NewDNSDomainResource,
		NewDNSRecordResource,
	}
}

func (p *hostkeyProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPresetDataSource,
		NewPresetsDataSource,
		NewOSesDataSource,
		NewTrafficPlansDataSource,
		NewSSHKeysDataSource,
		NewSoftwareDataSource,
		NewDNSDomainsDataSource,
	}
}
