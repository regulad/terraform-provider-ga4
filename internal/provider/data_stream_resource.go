package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
)

var (
	_ resource.Resource                     = (*dataStreamResource)(nil)
	_ resource.ResourceWithConfigure        = (*dataStreamResource)(nil)
	_ resource.ResourceWithImportState      = (*dataStreamResource)(nil)
	_ resource.ResourceWithConfigValidators = (*dataStreamResource)(nil)
	_ resource.ResourceWithValidateConfig   = (*dataStreamResource)(nil)
)

const (
	dataStreamTypeWeb     = "WEB_DATA_STREAM"
	dataStreamTypeAndroid = "ANDROID_APP_DATA_STREAM"
	dataStreamTypeIOS     = "IOS_APP_DATA_STREAM"
)

// dataStreamResource は GA4 のデータストリーム。
//
// It handles three kinds: web, Android app and iOS app. Which one is determined by which of `default_uri` (web),
// `android_app_stream_data` and `ios_app_stream_data` is written, and `type` is derived from that.
// Earlier versions handled only web streams, with `type` defaulting to `WEB_DATA_STREAM`, so existing configuration and state pass unchanged.
type dataStreamResource struct {
	svc *analyticsadmin.Service
}

type dataStreamResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	Property             types.String `tfsdk:"property"`
	Type                 types.String `tfsdk:"type"`
	DisplayName          types.String `tfsdk:"display_name"`
	DefaultURI           types.String `tfsdk:"default_uri"`
	AndroidAppStreamData types.Object `tfsdk:"android_app_stream_data"`
	IOSAppStreamData     types.Object `tfsdk:"ios_app_stream_data"`
	MeasurementID        types.String `tfsdk:"measurement_id"`
	FirebaseAppID        types.String `tfsdk:"firebase_app_id"`
}

type androidAppStreamDataModel struct {
	PackageName types.String `tfsdk:"package_name"`
}

type iosAppStreamDataModel struct {
	BundleID types.String `tfsdk:"bundle_id"`
}

var (
	androidAppStreamDataAttrTypes = map[string]attr.Type{"package_name": types.StringType}
	iosAppStreamDataAttrTypes     = map[string]attr.Type{"bundle_id": types.StringType}
)

func newDataStreamResource() resource.Resource {
	return &dataStreamResource{}
}

func (r *dataStreamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_data_stream"
}

func (r *dataStreamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A GA4 data stream. Write exactly one of web (`default_uri`), Android app (`android_app_stream_data`) " +
			"or iOS app (`ios_app_stream_data`). " +
			"To send through the Measurement Protocol, use `measurement_id` (web) or `firebase_app_id` (app) with `ga4_measurement_protocol_secret`'s `secret_value`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "ストリームのリソース名（`properties/123/dataStreams/456`）。`ga4_measurement_protocol_secret` の `data_stream` に渡す。",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"property": schema.StringAttribute{
				Required:      true,
				Description:   "所属するプロパティのリソース名（`properties/123`）。変更すると作り直しになる。",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "The kind of stream: `WEB_DATA_STREAM`, `ANDROID_APP_DATA_STREAM` or `IOS_APP_DATA_STREAM`. " +
					"If omitted, it is determined by the stream data written. If written, it must match that. Changing it forces replacement.",
				Validators: []validator.String{stringvalidator.OneOf(dataStreamTypeWeb, dataStreamTypeAndroid, dataStreamTypeIOS)},
				PlanModifiers: []planmodifier.String{
					dataStreamTypeFromConfig{},
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Required:    true,
				Description: "ストリームの表示名。",
			},
			"default_uri": schema.StringAttribute{
				Optional:    true,
				Description: "The web stream's URL (e.g. `https://app.example.com`). Write it for a web stream. If the stream is used only with the Measurement Protocol, the value has no effect.",
			},
			"android_app_stream_data": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Write it for an Android app stream. Changing it forces replacement.",
				PlanModifiers: []planmodifier.Object{objectplanmodifier.RequiresReplace()},
				Attributes: map[string]schema.Attribute{
					"package_name": schema.StringAttribute{
						Required:    true,
						Description: "The package name of the app to measure (e.g. `com.example.myandroidapp`).",
					},
				},
			},
			"ios_app_stream_data": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Write it for an iOS app stream. Changing it forces replacement.",
				PlanModifiers: []planmodifier.Object{objectplanmodifier.RequiresReplace()},
				Attributes: map[string]schema.Attribute{
					"bundle_id": schema.StringAttribute{
						Required:    true,
						Description: "The App Store bundle ID (e.g. `com.example.myiosapp`).",
					},
				},
			},
			"measurement_id": schema.StringAttribute{
				Computed:      true,
				Description:   "The measurement ID (`G-XXXXXXXXXX`). Web streams only; null for app streams.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"firebase_app_id": schema.StringAttribute{
				Computed:      true,
				Description:   "The ID of the corresponding Firebase app. For app streams, pass it as the Measurement Protocol's `firebase_app_id`. Usually empty for web streams.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

// ConfigValidators checks that exactly one kind of stream data is written
func (r *dataStreamResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("default_uri"),
			path.MatchRoot("android_app_stream_data"),
			path.MatchRoot("ios_app_stream_data"),
		),
	}
}

// ValidateConfig checks that a written `type` matches the stream data written
func (r *dataStreamResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config dataStreamResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Type.IsNull() || config.Type.IsUnknown() {
		return
	}

	want, ok := dataStreamTypeOf(config.AndroidAppStreamData, config.IOSAppStreamData)
	if !ok {
		return
	}
	if got := config.Type.ValueString(); got != want {
		resp.Diagnostics.AddAttributeError(
			path.Root("type"),
			"Data stream type mismatch",
			fmt.Sprintf("type is %q, but the stream data written is for %q. Remove type or make it match.", got, want),
		)
	}
}

// dataStreamTypeOf determines the kind from the stream data written. ok is false if either is unknown
func dataStreamTypeOf(android, ios types.Object) (string, bool) {
	if android.IsUnknown() || ios.IsUnknown() {
		return "", false
	}

	switch {
	case !android.IsNull():
		return dataStreamTypeAndroid, true
	case !ios.IsNull():
		return dataStreamTypeIOS, true
	default:
		return dataStreamTypeWeb, true
	}
}

// dataStreamTypeFromConfig, when `type` is not in the configuration, determines the kind from the stream data and puts it in the plan.
// Existing web stream configuration gets `WEB_DATA_STREAM`, the same as the earlier default, so no diff appears
type dataStreamTypeFromConfig struct{}

func (m dataStreamTypeFromConfig) Description(_ context.Context) string {
	return "If type is omitted, the kind is determined from the stream data written."
}

func (m dataStreamTypeFromConfig) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m dataStreamTypeFromConfig) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.ConfigValue.IsNull() {
		return
	}

	var android, ios types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("android_app_stream_data"), &android)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("ios_app_stream_data"), &ios)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if t, ok := dataStreamTypeOf(android, ios); ok {
		resp.PlanValue = types.StringValue(t)
	}
}

func (r *dataStreamResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if svc := serviceFromResourceConfigure(req, &resp.Diagnostics); svc != nil {
		r.svc = svc
	}
}

func (r *dataStreamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dataStreamResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := dataStreamFromPlan(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.svc.Properties.DataStreams.Create(plan.Property.ValueString(), body).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("GA4 データストリームを作成できません", err.Error())
		return
	}

	model, err := dataStreamModelFromAPI(created)
	if err != nil {
		resp.Diagnostics.AddError("GA4 データストリームの応答を読めません", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// dataStreamFromPlan builds the body that Create sends. The kind follows the plan's `type` (or, if it is not in the configuration, the value derived from the data)
func dataStreamFromPlan(ctx context.Context, plan dataStreamResourceModel) (*analyticsadmin.GoogleAnalyticsAdminV1betaDataStream, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &analyticsadmin.GoogleAnalyticsAdminV1betaDataStream{
		Type:        plan.Type.ValueString(),
		DisplayName: plan.DisplayName.ValueString(),
	}

	switch body.Type {
	case dataStreamTypeAndroid:
		var m androidAppStreamDataModel
		diags.Append(plan.AndroidAppStreamData.As(ctx, &m, basetypes.ObjectAsOptions{})...)
		body.AndroidAppStreamData = &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamAndroidAppStreamData{
			PackageName: m.PackageName.ValueString(),
		}
	case dataStreamTypeIOS:
		var m iosAppStreamDataModel
		diags.Append(plan.IOSAppStreamData.As(ctx, &m, basetypes.ObjectAsOptions{})...)
		body.IosAppStreamData = &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamIosAppStreamData{
			BundleId: m.BundleID.ValueString(),
		}
	default:
		body.WebStreamData = &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamWebStreamData{
			DefaultUri: plan.DefaultURI.ValueString(),
		}
	}

	return body, diags
}

func (r *dataStreamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dataStreamResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.svc.Properties.DataStreams.Get(state.ID.ValueString()).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("GA4 データストリームを取得できません", err.Error())
		return
	}

	model, err := dataStreamModelFromAPI(got)
	if err != nil {
		resp.Diagnostics.AddError("GA4 データストリームの応答を読めません", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *dataStreamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dataStreamResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, mask := dataStreamPatch(plan)

	updated, err := r.svc.Properties.DataStreams.Patch(state.ID.ValueString(), body).UpdateMask(mask).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("GA4 データストリームを更新できません", err.Error())
		return
	}

	model, err := dataStreamModelFromAPI(updated)
	if err != nil {
		resp.Diagnostics.AddError("GA4 データストリームの応答を読めません", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// dataStreamPatch builds the body and update_mask that Update sends.
// An app stream's package name or bundle ID cannot change (changing it forces replacement), so only the display name can change.
// A web stream's URL can change too
func dataStreamPatch(plan dataStreamResourceModel) (*analyticsadmin.GoogleAnalyticsAdminV1betaDataStream, string) {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaDataStream{
		DisplayName: plan.DisplayName.ValueString(),
	}
	if plan.Type.ValueString() != dataStreamTypeWeb {
		return body, "displayName"
	}

	body.WebStreamData = &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamWebStreamData{
		DefaultUri: plan.DefaultURI.ValueString(),
	}

	return body, "displayName,webStreamData.defaultUri"
}

func (r *dataStreamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dataStreamResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.svc.Properties.DataStreams.Delete(state.ID.ValueString()).Context(ctx).Do(); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("GA4 データストリームを削除できません", err.Error())
		return
	}
}

// ImportState は `terraform import ga4_data_stream.x properties/123/dataStreams/456` を受ける
func (r *dataStreamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// dataStreamModelFromAPI turns the API's response into state.
// A web stream gets the same values as before (default_uri, measurement_id and firebase_app_id are empty strings, not null, when empty),
// and the app stream attributes are null. For an app stream, default_uri and measurement_id are null
func dataStreamModelFromAPI(s *analyticsadmin.GoogleAnalyticsAdminV1betaDataStream) (*dataStreamResourceModel, error) {
	property, err := parentOfName(s.Name)
	if err != nil {
		return nil, err
	}

	model := &dataStreamResourceModel{
		ID:                   types.StringValue(s.Name),
		Property:             types.StringValue(property),
		Type:                 types.StringValue(s.Type),
		DisplayName:          types.StringValue(s.DisplayName),
		DefaultURI:           types.StringNull(),
		AndroidAppStreamData: types.ObjectNull(androidAppStreamDataAttrTypes),
		IOSAppStreamData:     types.ObjectNull(iosAppStreamDataAttrTypes),
		MeasurementID:        types.StringNull(),
	}

	switch s.Type {
	case dataStreamTypeAndroid:
		android := s.AndroidAppStreamData
		if android == nil {
			android = &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamAndroidAppStreamData{}
		}
		model.AndroidAppStreamData = types.ObjectValueMust(androidAppStreamDataAttrTypes, map[string]attr.Value{
			"package_name": types.StringValue(android.PackageName),
		})
		model.FirebaseAppID = types.StringValue(android.FirebaseAppId)
	case dataStreamTypeIOS:
		ios := s.IosAppStreamData
		if ios == nil {
			ios = &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamIosAppStreamData{}
		}
		model.IOSAppStreamData = types.ObjectValueMust(iosAppStreamDataAttrTypes, map[string]attr.Value{
			"bundle_id": types.StringValue(ios.BundleId),
		})
		model.FirebaseAppID = types.StringValue(ios.FirebaseAppId)
	default:
		web := s.WebStreamData
		if web == nil {
			web = &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamWebStreamData{}
		}
		model.DefaultURI = types.StringValue(web.DefaultUri)
		model.MeasurementID = types.StringValue(web.MeasurementId)
		model.FirebaseAppID = types.StringValue(web.FirebaseAppId)
	}

	return model, nil
}
