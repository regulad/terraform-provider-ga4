package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
)

var (
	_ resource.Resource                = (*dataRetentionSettingsResource)(nil)
	_ resource.ResourceWithConfigure   = (*dataRetentionSettingsResource)(nil)
	_ resource.ResourceWithImportState = (*dataRetentionSettingsResource)(nil)
)

// dataRetentionSettingsSuffix, appended to a property name, gives the resource name of its data retention settings
const dataRetentionSettingsSuffix = "/dataRetentionSettings"

// dataRetentionSettingsResource is a property's data retention settings.
//
// It is a singleton, one per property, which can be neither created nor deleted.
// Both create and update just call the update API, and destroy only removes it from state (the settings in GA4 stay as they are).
type dataRetentionSettingsResource struct {
	svc *analyticsadmin.Service
}

type dataRetentionSettingsResourceModel struct {
	ID                         types.String `tfsdk:"id"`
	Property                   types.String `tfsdk:"property"`
	EventDataRetention         types.String `tfsdk:"event_data_retention"`
	UserDataRetention          types.String `tfsdk:"user_data_retention"`
	ResetUserDataOnNewActivity types.Bool   `tfsdk:"reset_user_data_on_new_activity"`
}

func newDataRetentionSettingsResource() resource.Resource {
	return &dataRetentionSettingsResource{}
}

func (r *dataRetentionSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_data_retention_settings"
}

func (r *dataRetentionSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A GA4 property's data retention settings. Each property has exactly one, which can be neither created nor deleted. " +
			"create and update update the settings, and destroy only removes them from state (the settings in GA4 stay as they are).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The settings' resource name (`properties/123/dataRetentionSettings`).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"property": schema.StringAttribute{
				Required:      true,
				Description:   "The resource name of the property (`properties/123`). Changing it forces replacement (the old property's settings are removed from state unchanged).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"event_data_retention": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "How long event-level data is retained: `TWO_MONTHS`, `FOURTEEN_MONTHS`, " +
					"`TWENTY_SIX_MONTHS`, `THIRTY_EIGHT_MONTHS` or `FIFTY_MONTHS` (the last three on 360 properties only). If omitted, the value in GA4 is kept.",
				Validators: []validator.String{stringvalidator.OneOf(
					"TWO_MONTHS", "FOURTEEN_MONTHS", "TWENTY_SIX_MONTHS", "THIRTY_EIGHT_MONTHS", "FIFTY_MONTHS",
				)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"user_data_retention": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "How long user-level data is retained: `TWO_MONTHS` or `FOURTEEN_MONTHS`. If omitted, the value in GA4 is kept.",
				Validators:    []validator.String{stringvalidator.OneOf("TWO_MONTHS", "FOURTEEN_MONTHS")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"reset_user_data_on_new_activity": schema.BoolAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Whether to extend a user identifier's retention period with each new event from that user. If omitted, the value in GA4 is kept.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *dataRetentionSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if svc := serviceFromResourceConfigure(req, &resp.Diagnostics); svc != nil {
		r.svc = svc
	}
}

func (r *dataRetentionSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dataRetentionSettingsResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	model, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update GA4 data retention settings", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *dataRetentionSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dataRetentionSettingsResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.svc.Properties.GetDataRetentionSettings(state.ID.ValueString()).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read GA4 data retention settings", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, dataRetentionSettingsModelFromAPI(got))...)
}

func (r *dataRetentionSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dataRetentionSettingsResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	model, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update GA4 data retention settings", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// apply updates only what the configuration sets, and returns the settings after the update. Create and Update share it
func (r *dataRetentionSettingsResource) apply(ctx context.Context, plan dataRetentionSettingsResourceModel) (*dataRetentionSettingsResourceModel, error) {
	name := plan.Property.ValueString() + dataRetentionSettingsSuffix
	body, mask := dataRetentionSettingsPatch(plan)

	var (
		got *analyticsadmin.GoogleAnalyticsAdminV1betaDataRetentionSettings
		err error
	)
	if mask == "" {
		// If nothing is set, don't update; just read the current values (an empty update_mask updates nothing)
		got, err = r.svc.Properties.GetDataRetentionSettings(name).Context(ctx).Do()
	} else {
		got, err = r.svc.Properties.UpdateDataRetentionSettings(name, body).UpdateMask(mask).Context(ctx).Do()
	}
	if err != nil {
		return nil, err
	}

	return dataRetentionSettingsModelFromAPI(got), nil
}

// dataRetentionSettingsPatch builds the body and update_mask that the update sends.
// Every field is optional, so those not set (whose value the plan has not decided) keep the value in GA4
func dataRetentionSettingsPatch(plan dataRetentionSettingsResourceModel) (*analyticsadmin.GoogleAnalyticsAdminV1betaDataRetentionSettings, string) {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaDataRetentionSettings{}
	var mask []string

	if v := knownString(plan.EventDataRetention); v != "" {
		body.EventDataRetention = v
		mask = append(mask, "eventDataRetention")
	}
	if v := knownString(plan.UserDataRetention); v != "" {
		body.UserDataRetention = v
		mask = append(mask, "userDataRetention")
	}
	if !plan.ResetUserDataOnNewActivity.IsNull() && !plan.ResetUserDataOnNewActivity.IsUnknown() {
		// false must be sent too, so ForceSendFields keeps it from being dropped
		body.ResetUserDataOnNewActivity = plan.ResetUserDataOnNewActivity.ValueBool()
		body.ForceSendFields = append(body.ForceSendFields, "ResetUserDataOnNewActivity")
		mask = append(mask, "resetUserDataOnNewActivity")
	}

	return body, strings.Join(mask, ",")
}

// Delete only removes the settings from state. Data retention settings cannot be deleted, so the settings in GA4 stay as they are
func (r *dataRetentionSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState accepts `terraform import ga4_data_retention_settings.x properties/123`.
// `properties/123/dataRetentionSettings` is accepted too
func (r *dataRetentionSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	name, err := dataRetentionSettingsNameFromImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), name)...)
}

// dataRetentionSettingsNameFromImportID builds the settings' resource name from an import ID (a property name or the settings' resource name)
func dataRetentionSettingsNameFromImportID(id string) (string, error) {
	property := strings.TrimSuffix(id, dataRetentionSettingsSuffix)
	if err := checkResourceName(property, "properties"); err != nil {
		return "", err
	}

	return property + dataRetentionSettingsSuffix, nil
}

func dataRetentionSettingsModelFromAPI(s *analyticsadmin.GoogleAnalyticsAdminV1betaDataRetentionSettings) *dataRetentionSettingsResourceModel {
	return &dataRetentionSettingsResourceModel{
		ID:                         types.StringValue(s.Name),
		Property:                   types.StringValue(strings.TrimSuffix(s.Name, dataRetentionSettingsSuffix)),
		EventDataRetention:         types.StringValue(s.EventDataRetention),
		UserDataRetention:          types.StringValue(s.UserDataRetention),
		ResetUserDataOnNewActivity: types.BoolValue(s.ResetUserDataOnNewActivity),
	}
}
