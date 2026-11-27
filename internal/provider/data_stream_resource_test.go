package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
)

// Serves the provider as a protocol v6 server and calls its RPCs directly, in place of Terraform itself.
// plan, validate and import don't call the API (svc stays nil, as the provider is not configured), so these run without credentials.

func newTestServer(t *testing.T) tfprotov6.ProviderServer {
	t.Helper()

	server, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatalf("provider server: %v", err)
	}

	return server
}

// resourceType returns the type of the resource's schema
func resourceType(t *testing.T, server tfprotov6.ProviderServer, typeName string) tftypes.Object {
	t.Helper()

	resp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema: %v", err)
	}
	s, ok := resp.ResourceSchemas[typeName]
	if !ok {
		t.Fatalf("schema has no %s", typeName)
	}

	return s.ValueType().(tftypes.Object)
}

// objectValue builds an object with only the given attributes set and the rest null
func objectValue(typ tftypes.Object, attrs map[string]tftypes.Value) tftypes.Value {
	vals := map[string]tftypes.Value{}
	for name, attrType := range typ.AttributeTypes {
		if v, ok := attrs[name]; ok {
			vals[name] = v
		} else {
			vals[name] = tftypes.NewValue(attrType, nil)
		}
	}

	return tftypes.NewValue(typ, vals)
}

func dynamicValue(t *testing.T, typ tftypes.Type, v tftypes.Value) *tfprotov6.DynamicValue {
	t.Helper()

	dv, err := tfprotov6.NewDynamicValue(typ, v)
	if err != nil {
		t.Fatalf("NewDynamicValue: %v", err)
	}

	return &dv
}

func str(s string) tftypes.Value {
	return tftypes.NewValue(tftypes.String, s)
}

func hasError(diags []*tfprotov6.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			return true
		}
	}

	return false
}

// Checks that state and configuration written by the earlier version (web streams only, `type` defaulting to WEB_DATA_STREAM)
// produce neither a diff nor a replacement
func TestDataStreamBackwardCompatiblePlan(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t)
	typ := resourceType(t, server, "ga4_data_stream")

	// The earlier version's state. It has no android_app_stream_data or ios_app_stream_data
	oldState := []byte(`{"id":"properties/1/dataStreams/2","property":"properties/1","type":"WEB_DATA_STREAM",` +
		`"display_name":"My App (web)","default_uri":"https://example.com","measurement_id":"G-ABC","firebase_app_id":""}`)

	upgraded, err := server.UpgradeResourceState(ctx, &tfprotov6.UpgradeResourceStateRequest{
		TypeName: "ga4_data_stream",
		Version:  0,
		RawState: &tfprotov6.RawState{JSON: oldState},
	})
	if err != nil || hasError(upgraded.Diagnostics) {
		t.Fatalf("UpgradeResourceState: %v %v", err, upgraded.Diagnostics)
	}
	prior, err := upgraded.UpgradedState.Unmarshal(typ)
	if err != nil {
		t.Fatalf("upgraded state: %v", err)
	}

	cases := map[string]map[string]tftypes.Value{
		"type omitted": {
			"property":     str("properties/1"),
			"display_name": str("My App (web)"),
			"default_uri":  str("https://example.com"),
		},
		"type explicit": {
			"property":     str("properties/1"),
			"type":         str("WEB_DATA_STREAM"),
			"display_name": str("My App (web)"),
			"default_uri":  str("https://example.com"),
		},
	}

	for name, attrs := range cases {
		t.Run(name, func(t *testing.T) {
			config := objectValue(typ, attrs)

			// Computed attributes absent from the configuration carry over the prior state's values (built as Terraform's proposed new state is)
			proposed, err := tftypes.Transform(config, func(p *tftypes.AttributePath, v tftypes.Value) (tftypes.Value, error) {
				if len(p.Steps()) != 1 || !v.IsNull() {
					return v, nil
				}
				priorValue, _, err := tftypes.WalkAttributePath(prior, p)
				if err != nil {
					return v, nil
				}
				return priorValue.(tftypes.Value), nil
			})
			if err != nil {
				t.Fatalf("proposed: %v", err)
			}

			resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
				TypeName:         "ga4_data_stream",
				PriorState:       upgraded.UpgradedState,
				ProposedNewState: dynamicValue(t, typ, proposed),
				Config:           dynamicValue(t, typ, config),
			})
			if err != nil || hasError(resp.Diagnostics) {
				t.Fatalf("PlanResourceChange: %v %v", err, resp.Diagnostics)
			}
			if len(resp.RequiresReplace) != 0 {
				t.Errorf("requires replacement: %v", resp.RequiresReplace)
			}

			planned, err := resp.PlannedState.Unmarshal(typ)
			if err != nil {
				t.Fatalf("planned state: %v", err)
			}
			if diffs, err := planned.Diff(prior); err != nil || len(diffs) != 0 {
				t.Errorf("has a diff: %v %v", diffs, err)
			}
		})
	}
}

// For a newly created app stream, `type` is determined by the stream's data
func TestDataStreamPlanDerivesType(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t)
	typ := resourceType(t, server, "ga4_data_stream")
	androidType := typ.AttributeTypes["android_app_stream_data"].(tftypes.Object)
	iosType := typ.AttributeTypes["ios_app_stream_data"].(tftypes.Object)

	cases := []struct {
		name  string
		attrs map[string]tftypes.Value
		want  string
	}{
		{
			name: "web",
			attrs: map[string]tftypes.Value{
				"default_uri": str("https://example.com"),
			},
			want: dataStreamTypeWeb,
		},
		{
			name: "android",
			attrs: map[string]tftypes.Value{
				"android_app_stream_data": tftypes.NewValue(androidType, map[string]tftypes.Value{"package_name": str("com.example.app")}),
			},
			want: dataStreamTypeAndroid,
		},
		{
			name: "ios",
			attrs: map[string]tftypes.Value{
				"ios_app_stream_data": tftypes.NewValue(iosType, map[string]tftypes.Value{"bundle_id": str("com.example.app")}),
			},
			want: dataStreamTypeIOS,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.attrs["property"] = str("properties/1")
			c.attrs["display_name"] = str("App")
			config := objectValue(typ, c.attrs)

			resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
				TypeName:         "ga4_data_stream",
				PriorState:       dynamicValue(t, typ, tftypes.NewValue(typ, nil)),
				ProposedNewState: dynamicValue(t, typ, config),
				Config:           dynamicValue(t, typ, config),
			})
			if err != nil || hasError(resp.Diagnostics) {
				t.Fatalf("PlanResourceChange: %v %v", err, resp.Diagnostics)
			}

			planned, err := resp.PlannedState.Unmarshal(typ)
			if err != nil {
				t.Fatalf("planned state: %v", err)
			}
			v, _, err := tftypes.WalkAttributePath(planned, tftypes.NewAttributePath().WithAttributeName("type"))
			if err != nil {
				t.Fatalf("type: %v", err)
			}
			var got string
			if err := v.(tftypes.Value).As(&got); err != nil {
				t.Fatalf("type: %v", err)
			}
			if got != c.want {
				t.Errorf("type = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDataStreamValidateConfig(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t)
	typ := resourceType(t, server, "ga4_data_stream")
	androidType := typ.AttributeTypes["android_app_stream_data"].(tftypes.Object)
	android := tftypes.NewValue(androidType, map[string]tftypes.Value{"package_name": str("com.example.app")})

	cases := []struct {
		name    string
		attrs   map[string]tftypes.Value
		wantErr bool
	}{
		{name: "web only", attrs: map[string]tftypes.Value{"default_uri": str("https://example.com")}},
		{name: "android only", attrs: map[string]tftypes.Value{"android_app_stream_data": android}},
		{name: "none", attrs: map[string]tftypes.Value{}, wantErr: true},
		{
			name:    "both web and android",
			attrs:   map[string]tftypes.Value{"default_uri": str("https://example.com"), "android_app_stream_data": android},
			wantErr: true,
		},
		{
			name:    "type mismatch",
			attrs:   map[string]tftypes.Value{"type": str(dataStreamTypeWeb), "android_app_stream_data": android},
			wantErr: true,
		},
		{
			name:  "type matches",
			attrs: map[string]tftypes.Value{"type": str(dataStreamTypeAndroid), "android_app_stream_data": android},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.attrs["property"] = str("properties/1")
			c.attrs["display_name"] = str("App")

			resp, err := server.ValidateResourceConfig(ctx, &tfprotov6.ValidateResourceConfigRequest{
				TypeName: "ga4_data_stream",
				Config:   dynamicValue(t, typ, objectValue(typ, c.attrs)),
			})
			if err != nil {
				t.Fatalf("ValidateResourceConfig: %v", err)
			}
			if got := hasError(resp.Diagnostics); got != c.wantErr {
				t.Errorf("error = %v, want %v (%v)", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestDataStreamModelFromAPI(t *testing.T) {
	t.Run("web", func(t *testing.T) {
		m, err := dataStreamModelFromAPI(&analyticsadmin.GoogleAnalyticsAdminV1betaDataStream{
			Name:          "properties/1/dataStreams/2",
			Type:          dataStreamTypeWeb,
			DisplayName:   "web",
			WebStreamData: &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamWebStreamData{DefaultUri: "https://example.com", MeasurementId: "G-ABC"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if m.Property.ValueString() != "properties/1" || m.DefaultURI.ValueString() != "https://example.com" || m.MeasurementID.ValueString() != "G-ABC" {
			t.Errorf("wrong web values: %+v", m)
		}
		// As in the earlier version, an empty firebase_app_id is an empty string, not null
		if m.FirebaseAppID.IsNull() || m.FirebaseAppID.ValueString() != "" {
			t.Errorf("firebase_app_id = %v, want empty string", m.FirebaseAppID)
		}
		if !m.AndroidAppStreamData.IsNull() || !m.IOSAppStreamData.IsNull() {
			t.Error("web stream has app stream data")
		}
	})

	t.Run("android", func(t *testing.T) {
		m, err := dataStreamModelFromAPI(&analyticsadmin.GoogleAnalyticsAdminV1betaDataStream{
			Name: "properties/1/dataStreams/3",
			Type: dataStreamTypeAndroid,
			AndroidAppStreamData: &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamAndroidAppStreamData{
				PackageName: "com.example.app", FirebaseAppId: "1:2:android:3",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !m.DefaultURI.IsNull() || !m.MeasurementID.IsNull() || !m.IOSAppStreamData.IsNull() {
			t.Errorf("android stream has web or iOS values: %+v", m)
		}
		if got := m.AndroidAppStreamData.Attributes()["package_name"]; got.String() != `"com.example.app"` {
			t.Errorf("package_name = %v", got)
		}
		if m.FirebaseAppID.ValueString() != "1:2:android:3" {
			t.Errorf("firebase_app_id = %v", m.FirebaseAppID)
		}
	})

	t.Run("ios", func(t *testing.T) {
		m, err := dataStreamModelFromAPI(&analyticsadmin.GoogleAnalyticsAdminV1betaDataStream{
			Name: "properties/1/dataStreams/4",
			Type: dataStreamTypeIOS,
			IosAppStreamData: &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamIosAppStreamData{
				BundleId: "com.example.app", FirebaseAppId: "1:2:ios:3",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := m.IOSAppStreamData.Attributes()["bundle_id"]; got.String() != `"com.example.app"` {
			t.Errorf("bundle_id = %v", got)
		}
		if m.FirebaseAppID.ValueString() != "1:2:ios:3" || !m.AndroidAppStreamData.IsNull() {
			t.Errorf("wrong ios values: %+v", m)
		}
	})
}

func TestDataStreamPatch(t *testing.T) {
	web := dataStreamResourceModel{}
	web.Type = types.StringValue(dataStreamTypeWeb)
	web.DisplayName = types.StringValue("web")
	web.DefaultURI = types.StringValue("https://example.com")

	body, mask := dataStreamPatch(web)
	if mask != "displayName,webStreamData.defaultUri" || body.WebStreamData == nil || body.WebStreamData.DefaultUri != "https://example.com" {
		t.Errorf("web: mask = %q, body = %+v", mask, body)
	}

	android := dataStreamResourceModel{}
	android.Type = types.StringValue(dataStreamTypeAndroid)
	android.DisplayName = types.StringValue("android")

	body, mask = dataStreamPatch(android)
	if mask != "displayName" || body.WebStreamData != nil || body.AndroidAppStreamData != nil {
		t.Errorf("android: mask = %q, body = %+v", mask, body)
	}
}
