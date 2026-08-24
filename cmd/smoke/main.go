package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

// Smoke-test InvAPI without Terraform.
//
//	set HOSTKEY_API_KEY=your-key
//	go run ./cmd/smoke
//	go run ./cmd/smoke -preset <id-from-presets-list>
//	go run ./cmd/smoke -server <your-server-id>
//	go run ./cmd/smoke -base-url https://invapi-stage.hostkey.com/
func main() {
	var (
		baseURL  = flag.String("base-url", "", "InvAPI base URL override")
		presetID = flag.Int("preset", 0, "presets/show id")
		serverID = flag.Int("server", 0, "eq/show id")
	)
	flag.Parse()

	apiKey := os.Getenv("HOSTKEY_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "HOSTKEY_API_KEY is required")
		os.Exit(1)
	}

	urlBase := *baseURL
	if urlBase == "" {
		urlBase = os.Getenv("HOSTKEY_BASE_URL")
	}
	if urlBase == "" {
		urlBase = invapi.DefaultBaseURL
	}

	ctx := context.Background()

	client, err := invapi.NewClient(invapi.Config{BaseURL: urlBase}, nil)
	if err != nil {
		fail("client", err)
	}

	auth := invapi.NewTokenManager(apiKey, 3600, client)
	client.SetAuth(auth)

	token, err := auth.Token(ctx)
	if err != nil {
		fail("auth/login", err)
	}
	prefix := token
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	fmt.Printf("OK auth/login  base=%s  token=%s…\n", client.BaseURL(), prefix)

	if *presetID > 0 {
		params := url.Values{}
		params.Set("action", "show")
		params.Set("id", strconv.Itoa(*presetID))
		body, err := client.PostForm(ctx, "presets", params)
		if err != nil {
			fail("presets/show", err)
		}
		fmt.Printf("OK presets/show id=%d\n%s\n", *presetID, truncate(body, 500))
	}

	if *serverID > 0 {
		show, err := client.EQShow(ctx, *serverID)
		if err != nil {
			fail("eq/show", err)
		}
		fmt.Printf("OK eq/show id=%d result=%s main_ip=%s\n",
			*serverID, show.Result, invapi.MainIPv4(show))
	}

	fmt.Println("\nSmoke test passed.")
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", step, err)
	os.Exit(1)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
