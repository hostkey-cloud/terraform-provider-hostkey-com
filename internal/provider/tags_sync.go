package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

func mapStringValues(m types.Map) map[string]string {
	out := map[string]string{}
	if m.IsNull() || m.IsUnknown() {
		return out
	}
	raw := map[string]types.String{}
	_ = m.ElementsAs(context.Background(), &raw, false)
	for k, v := range raw {
		out[k] = v.ValueString()
	}
	return out
}

func tagsMapValue(tags map[string]string) types.Map {
	if len(tags) == 0 {
		return types.MapNull(types.StringType)
	}
	elems := make(map[string]attr.Value, len(tags))
	for k, v := range tags {
		elems[k] = types.StringValue(v)
	}
	return types.MapValueMust(types.StringType, elems)
}

func isProtectedTag(name string) bool {
	switch strings.ToLower(name) {
	case "password", "os", "hostname", "root_pass", "rootpassword":
		return true
	default:
		return false
	}
}

func (r *serverResource) readUserTags(ctx context.Context, serverID int) (map[string]string, error) {
	list, err := r.client.TagsList(ctx, serverID)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, t := range list.Tags {
		if t.Internal != 0 || t.Tag == "" || isProtectedTag(t.Tag) {
			continue
		}
		out[t.Tag] = t.Value
	}
	return out, nil
}

func filterConfiguredTags(configured types.Map, live map[string]string) types.Map {
	want := mapStringValues(configured)
	if len(want) == 0 {
		return types.MapNull(types.StringType)
	}
	out := make(map[string]string, len(want))
	for k, v := range want {
		if isProtectedTag(k) {
			continue
		}
		if lv, ok := live[k]; ok {
			out[k] = lv
		} else {
			out[k] = v
		}
	}
	return tagsMapValue(out)
}

// syncTags applies desired user tags. previous is the last known Terraform tags map (may be null).
// Only keys present in Terraform config are added/updated/removed — never Hostkey system tags.
func (r *serverResource) syncTags(ctx context.Context, serverID int, desired, previous types.Map) error {
	want := mapStringValues(desired)
	had := mapStringValues(previous)

	live, _ := r.readUserTags(ctx, serverID)

	for name, val := range want {
		if isProtectedTag(name) {
			continue
		}
		if cur, ok := live[name]; ok && cur == val {
			continue
		}
		if err := r.client.TagsAdd(ctx, serverID, name, val); err != nil {
			return fmt.Errorf("add tag %q: %w", name, err)
		}
	}

	for name := range had {
		if isProtectedTag(name) {
			continue
		}
		if _, keep := want[name]; keep {
			continue
		}
		if _, exists := live[name]; !exists {
			continue
		}
		if err := r.client.TagsRemove(ctx, serverID, name); err != nil {
			return fmt.Errorf("remove tag %q: %w", name, err)
		}
	}
	return nil
}

func serverStatus(show *invapi.ServerShowResponse) string {
	if show == nil {
		return ""
	}
	var sd map[string]any
	if err := json.Unmarshal(show.ServerData, &sd); err == nil {
		if v, ok := sd["status"].(string); ok && v != "" {
			return v
		}
		if v, ok := sd["Condition_Component"].(string); ok && v != "" {
			return v
		}
	}
	return strings.TrimSpace(show.Result)
}
