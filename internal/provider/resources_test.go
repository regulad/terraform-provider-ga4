package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
)

// requestJSON returns the body (JSON) sent to the API as a map. Used to check that ForceSendFields takes effect
func requestJSON(t *testing.T, v any) map[string]any {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	return m
}

func TestCheckResourceName(t *testing.T) {
	cases := []struct {
		name        string
		collections []string
		wantErr     bool
	}{
		{name: "accounts/123", collections: []string{"accounts"}},
		{name: "properties/123/keyEvents/456", collections: []string{"properties", "keyEvents"}},
		{name: "properties/123/keyEvents/456", collections: []string{"properties", "customMetrics"}, wantErr: true},
		{name: "properties/123", collections: []string{"properties", "keyEvents"}, wantErr: true},
		{name: "properties/123/keyEvents/", collections: []string{"properties", "keyEvents"}, wantErr: true},
		{name: "123", collections: []string{"accounts"}, wantErr: true},
		{name: "", collections: []string{"accounts"}, wantErr: true},
	}

	for _, c := range cases {
		err := checkResourceName(c.name, c.collections...)
		if (err != nil) != c.wantErr {
			t.Errorf("checkResourceName(%q, %v) = %v, wantErr %v", c.name, c.collections, err, c.wantErr)
		}
	}

	if got := exampleResourceName("properties", "dataStreams", "measurementProtocolSecrets"); got != "properties/123/dataStreams/456/measurementProtocolSecrets/789" {
		t.Errorf("exampleResourceName = %q", got)
	}
}

func TestDataRetentionSettingsNameFromImportID(t *testing.T) {
	cases := []struct {
		id      string
		want    string
		wantErr bool
	}{
		{id: "properties/123", want: "properties/123/dataRetentionSettings"},
		{id: "properties/123/dataRetentionSettings", want: "properties/123/dataRetentionSettings"},
		{id: "123", wantErr: true},
		{id: "properties/123/dataStreams/456", wantErr: true},
	}

	for _, c := range cases {
		got, err := dataRetentionSettingsNameFromImportID(c.id)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("dataRetentionSettingsNameFromImportID(%q) = %q, %v; want %q, wantErr %v", c.id, got, err, c.want, c.wantErr)
		}
	}
}

// An import ID's form is checked before Read, and if valid it goes into id unchanged
func TestImportState(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t)

	cases := []struct {
		typeName string
		id       string
		wantID   string
		wantErr  bool
	}{
		{typeName: "ga4_account", id: "accounts/123", wantID: "accounts/123"},
		{typeName: "ga4_account", id: "123", wantErr: true},
		{typeName: "ga4_data_retention_settings", id: "properties/123", wantID: "properties/123/dataRetentionSettings"},
		{typeName: "ga4_custom_metric", id: "properties/123/customMetrics/456", wantID: "properties/123/customMetrics/456"},
		{typeName: "ga4_custom_metric", id: "properties/123/customDimensions/456", wantErr: true},
		{typeName: "ga4_key_event", id: "properties/123/keyEvents/456", wantID: "properties/123/keyEvents/456"},
		{typeName: "ga4_google_ads_link", id: "properties/123/googleAdsLinks/456", wantID: "properties/123/googleAdsLinks/456"},
		{typeName: "ga4_firebase_link", id: "properties/123/firebaseLinks/456", wantID: "properties/123/firebaseLinks/456"},
		{typeName: "ga4_firebase_link", id: "properties/123", wantErr: true},
	}

	for _, c := range cases {
		resp, err := server.ImportResourceState(ctx, &tfprotov6.ImportResourceStateRequest{TypeName: c.typeName, ID: c.id})
		if err != nil {
			t.Fatalf("%s %q: %v", c.typeName, c.id, err)
		}
		if got := hasError(resp.Diagnostics); got != c.wantErr {
			t.Errorf("%s %q: error = %v, want %v (%v)", c.typeName, c.id, got, c.wantErr, resp.Diagnostics)
			continue
		}
		if c.wantErr {
			continue
		}

		typ := resourceType(t, server, c.typeName)
		state, err := resp.ImportedResources[0].State.Unmarshal(typ)
		if err != nil {
			t.Fatalf("%s: %v", c.typeName, err)
		}
		v, _, err := tftypes.WalkAttributePath(state, tftypes.NewAttributePath().WithAttributeName("id"))
		if err != nil {
			t.Fatalf("%s: %v", c.typeName, err)
		}
		var got string
		if err := v.(tftypes.Value).As(&got); err != nil {
			t.Fatalf("%s: %v", c.typeName, err)
		}
		if got != c.wantID {
			t.Errorf("%s %q: id = %q, want %q", c.typeName, c.id, got, c.wantID)
		}
	}
}

// An imported property is treated as not acknowledged: acknowledge_user_data_collection is false.
func TestPropertyImportStateAcknowledgement(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t)

	resp, err := server.ImportResourceState(ctx, &tfprotov6.ImportResourceStateRequest{TypeName: "ga4_property", ID: "properties/123"})
	if err != nil {
		t.Fatal(err)
	}
	if hasError(resp.Diagnostics) {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	state, err := resp.ImportedResources[0].State.Unmarshal(resourceType(t, server, "ga4_property"))
	if err != nil {
		t.Fatal(err)
	}
	v, _, err := tftypes.WalkAttributePath(state, tftypes.NewAttributePath().WithAttributeName("acknowledge_user_data_collection"))
	if err != nil {
		t.Fatal(err)
	}
	var got *bool
	if err := v.(tftypes.Value).As(&got); err != nil {
		t.Fatal(err)
	}
	if got == nil || *got {
		t.Errorf("acknowledge_user_data_collection = %v, want false", got)
	}
}

func TestAccountPatch(t *testing.T) {
	plan := accountResourceModel{DisplayName: types.StringValue("My Company"), RegionCode: types.StringUnknown()}
	body, mask := accountPatch(plan)
	if mask != "displayName" || body.RegionCode != "" {
		t.Errorf("region_code omitted: mask = %q, body = %+v", mask, body)
	}

	plan.RegionCode = types.StringValue("JP")
	body, mask = accountPatch(plan)
	if mask != "displayName,regionCode" || body.DisplayName != "My Company" || body.RegionCode != "JP" {
		t.Errorf("region_code set: mask = %q, body = %+v", mask, body)
	}
}

func TestAccountModelFromAPI(t *testing.T) {
	m := accountModelFromAPI(&analyticsadmin.GoogleAnalyticsAdminV1betaAccount{
		Name: "accounts/1", DisplayName: "My Company", RegionCode: "JP",
	})
	if m.ID.ValueString() != "accounts/1" || m.DisplayName.ValueString() != "My Company" || m.RegionCode.ValueString() != "JP" {
		t.Errorf("wrong values: %+v", m)
	}
	if m.GmpOrganization.IsNull() {
		t.Error("gmp_organization must not be null even when empty (so the Computed value doesn't change on every run)")
	}
}

func TestDataRetentionSettingsPatch(t *testing.T) {
	// If nothing is set, nothing is updated
	_, mask := dataRetentionSettingsPatch(dataRetentionSettingsResourceModel{
		EventDataRetention:         types.StringUnknown(),
		UserDataRetention:          types.StringNull(),
		ResetUserDataOnNewActivity: types.BoolUnknown(),
	})
	if mask != "" {
		t.Errorf("nothing set, but mask = %q", mask)
	}

	body, mask := dataRetentionSettingsPatch(dataRetentionSettingsResourceModel{
		EventDataRetention:         types.StringValue("FOURTEEN_MONTHS"),
		UserDataRetention:          types.StringValue("TWO_MONTHS"),
		ResetUserDataOnNewActivity: types.BoolValue(false),
	})
	if mask != "eventDataRetention,userDataRetention,resetUserDataOnNewActivity" {
		t.Errorf("mask = %q", mask)
	}
	got := requestJSON(t, body)
	if got["eventDataRetention"] != "FOURTEEN_MONTHS" || got["userDataRetention"] != "TWO_MONTHS" {
		t.Errorf("body = %v", got)
	}
	// false is sent too
	if v, ok := got["resetUserDataOnNewActivity"]; !ok || v != false {
		t.Errorf("resetUserDataOnNewActivity = %v (not sent)", v)
	}
}

func TestDataRetentionSettingsModelFromAPI(t *testing.T) {
	m := dataRetentionSettingsModelFromAPI(&analyticsadmin.GoogleAnalyticsAdminV1betaDataRetentionSettings{
		Name:                       "properties/1/dataRetentionSettings",
		EventDataRetention:         "FOURTEEN_MONTHS",
		UserDataRetention:          "FOURTEEN_MONTHS",
		ResetUserDataOnNewActivity: true,
	})
	if m.ID.ValueString() != "properties/1/dataRetentionSettings" || m.Property.ValueString() != "properties/1" {
		t.Errorf("wrong id or property: %+v", m)
	}
	if m.EventDataRetention.ValueString() != "FOURTEEN_MONTHS" || !m.ResetUserDataOnNewActivity.ValueBool() {
		t.Errorf("wrong values: %+v", m)
	}
}

func TestCustomMetricPatch(t *testing.T) {
	ctx := context.Background()
	plan := customMetricResourceModel{
		DisplayName:          types.StringValue("Score"),
		Description:          types.StringValue(""),
		MeasurementUnit:      types.StringValue("STANDARD"),
		RestrictedMetricType: types.SetValueMust(types.StringType, []attr.Value{}),
	}

	body, mask, diags := customMetricPatch(ctx, plan)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if mask != "displayName,description,measurementUnit,restrictedMetricType" {
		t.Errorf("mask = %q", mask)
	}
	got := requestJSON(t, body)
	// Fields are sent even when cleared
	if v, ok := got["description"]; !ok || v != "" {
		t.Errorf("description = %v (not sent)", v)
	}
	if v, ok := got["restrictedMetricType"]; !ok || len(v.([]any)) != 0 {
		t.Errorf("restrictedMetricType = %v (not sent empty)", v)
	}
	if _, ok := got["parameterName"]; ok {
		t.Error("sends parameterName, which cannot change")
	}

	plan.MeasurementUnit = types.StringValue("CURRENCY")
	plan.RestrictedMetricType = types.SetValueMust(types.StringType, []attr.Value{types.StringValue("REVENUE_DATA")})
	body, _, _ = customMetricPatch(ctx, plan)
	if len(body.RestrictedMetricType) != 1 || body.RestrictedMetricType[0] != "REVENUE_DATA" {
		t.Errorf("restrictedMetricType = %v", body.RestrictedMetricType)
	}
}

func TestCustomMetricModelFromAPI(t *testing.T) {
	m, err := customMetricModelFromAPI(&analyticsadmin.GoogleAnalyticsAdminV1betaCustomMetric{
		Name:            "properties/1/customMetrics/2",
		ParameterName:   "score",
		DisplayName:     "Score",
		MeasurementUnit: "STANDARD",
		Scope:           "EVENT",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Property.ValueString() != "properties/1" || m.ParameterName.ValueString() != "score" || m.Scope.ValueString() != "EVENT" {
		t.Errorf("wrong values: %+v", m)
	}
	// The API omits an empty restrictedMetricType, so it must match the default empty set
	if m.RestrictedMetricType.IsNull() || len(m.RestrictedMetricType.Elements()) != 0 {
		t.Errorf("restricted_metric_type = %v, want empty set", m.RestrictedMetricType)
	}
	if m.Description.IsNull() {
		t.Error("description must not be null even when empty (to match the default empty string)")
	}
}

func TestKeyEventPatch(t *testing.T) {
	ctx := context.Background()

	// When default_value is removed, it goes in the mask only, not the body, which clears it
	body, mask, diags := keyEventPatch(ctx, keyEventResourceModel{
		CountingMethod: types.StringValue("ONCE_PER_EVENT"),
		DefaultValue:   types.ObjectNull(keyEventDefaultValueAttrTypes),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if mask != "countingMethod,defaultValue" || body.DefaultValue != nil {
		t.Errorf("no default_value: mask = %q, body = %+v", mask, body)
	}
	if _, ok := requestJSON(t, body)["eventName"]; ok {
		t.Error("sends eventName, which cannot change")
	}

	// 0 is sent as a default too
	body, _, diags = keyEventPatch(ctx, keyEventResourceModel{
		CountingMethod: types.StringValue("ONCE_PER_SESSION"),
		DefaultValue: types.ObjectValueMust(keyEventDefaultValueAttrTypes, map[string]attr.Value{
			"numeric_value": types.Float64Value(0),
			"currency_code": types.StringValue("JPY"),
		}),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	dv, ok := requestJSON(t, body)["defaultValue"].(map[string]any)
	if !ok {
		t.Fatalf("defaultValue not sent: %+v", body)
	}
	if v, ok := dv["numericValue"]; !ok || v != float64(0) {
		t.Errorf("numericValue = %v (0 not sent)", v)
	}
	if dv["currencyCode"] != "JPY" {
		t.Errorf("currencyCode = %v", dv["currencyCode"])
	}
}

func TestKeyEventModelFromAPI(t *testing.T) {
	m, err := keyEventModelFromAPI(&analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent{
		Name:           "properties/1/keyEvents/2",
		EventName:      "purchase",
		CountingMethod: "ONCE_PER_EVENT",
		Deletable:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Property.ValueString() != "properties/1" || m.EventName.ValueString() != "purchase" || !m.Deletable.ValueBool() || m.Custom.ValueBool() {
		t.Errorf("wrong values: %+v", m)
	}
	if !m.DefaultValue.IsNull() {
		t.Errorf("default_value = %v, want null", m.DefaultValue)
	}

	m, err = keyEventModelFromAPI(&analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent{
		Name:         "properties/1/keyEvents/3",
		DefaultValue: &analyticsadmin.GoogleAnalyticsAdminV1betaKeyEventDefaultValue{NumericValue: 1.5, CurrencyCode: "JPY"},
	})
	if err != nil {
		t.Fatal(err)
	}
	attrs := m.DefaultValue.Attributes()
	if attrs["numeric_value"].(types.Float64).ValueFloat64() != 1.5 || attrs["currency_code"].(types.String).ValueString() != "JPY" {
		t.Errorf("default_value = %v", m.DefaultValue)
	}
}

func TestGoogleAdsLinkModelFromAPI(t *testing.T) {
	links := []*analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink{
		{Name: "properties/1/googleAdsLinks/2", CustomerId: "1112223333"},
		{Name: "properties/1/googleAdsLinks/3", CustomerId: "4445556666", AdsPersonalizationEnabled: true, CanManageClients: true},
	}

	found := findGoogleAdsLinkByName(links, "properties/1/googleAdsLinks/3")
	if found == nil || found.CustomerId != "4445556666" {
		t.Fatalf("not found in the list: %+v", found)
	}
	if findGoogleAdsLinkByName(links, "properties/1/googleAdsLinks/9") != nil {
		t.Error("found a link that doesn't exist")
	}

	m, err := googleAdsLinkModelFromAPI(found)
	if err != nil {
		t.Fatal(err)
	}
	if m.Property.ValueString() != "properties/1" || m.CustomerID.ValueString() != "4445556666" ||
		!m.AdsPersonalizationEnabled.ValueBool() || !m.CanManageClients.ValueBool() {
		t.Errorf("wrong values: %+v", m)
	}
}

func TestFirebaseLinkModelFromAPI(t *testing.T) {
	link := &analyticsadmin.GoogleAnalyticsAdminV1betaFirebaseLink{Name: "properties/1/firebaseLinks/2", Project: "projects/1234"}

	cases := []struct {
		name  string
		known string
		want  string
	}{
		// Right after an import, the API's value (in the number form)
		{name: "import", known: "", want: "projects/1234"},
		// Written in the ID form, the value is kept (the API returns only numbers)
		{name: "ID form", known: "projects/my-project", want: "projects/my-project"},
		{name: "number form", known: "projects/1234", want: "projects/1234"},
		// A different number means it was relinked elsewhere, so the API's value
		{name: "different number", known: "projects/9999", want: "projects/1234"},
	}

	for _, c := range cases {
		m, err := firebaseLinkModelFromAPI(link, c.known)
		if err != nil {
			t.Fatal(err)
		}
		if m.Project.ValueString() != c.want || m.Property.ValueString() != "properties/1" {
			t.Errorf("%s: project = %q, want %q", c.name, m.Project.ValueString(), c.want)
		}
	}
}

// Firebase links have no update, so changing any argument forces replacement
func TestFirebaseLinkRequiresReplace(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t)
	typ := resourceType(t, server, "ga4_firebase_link")

	prior := objectValue(typ, map[string]tftypes.Value{
		"id": str("properties/1/firebaseLinks/2"), "property": str("properties/1"), "project": str("projects/1"),
	})

	changed := map[string]string{"property": "properties/2", "project": "projects/2"}

	for attrName, value := range changed {
		attrs := map[string]tftypes.Value{"property": str("properties/1"), "project": str("projects/1")}
		attrs[attrName] = str(value)
		config := objectValue(typ, attrs)
		proposedAttrs := map[string]tftypes.Value{"id": str("properties/1/firebaseLinks/2")}
		for k, v := range attrs {
			proposedAttrs[k] = v
		}

		resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
			TypeName:         "ga4_firebase_link",
			PriorState:       dynamicValue(t, typ, prior),
			ProposedNewState: dynamicValue(t, typ, objectValue(typ, proposedAttrs)),
			Config:           dynamicValue(t, typ, config),
		})
		if err != nil || hasError(resp.Diagnostics) {
			t.Fatalf("%s: %v %v", attrName, err, resp.Diagnostics)
		}
		if len(resp.RequiresReplace) == 0 {
			t.Errorf("changing %s does not force replacement", attrName)
		}
	}
}
