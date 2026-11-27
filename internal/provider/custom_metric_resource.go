package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
)

var (
	_ resource.Resource                = (*customMetricResource)(nil)
	_ resource.ResourceWithConfigure   = (*customMetricResource)(nil)
	_ resource.ResourceWithImportState = (*customMetricResource)(nil)
)

// customMetricResource is a GA4 custom metric.
//
// As with custom dimensions, GA4 has no delete, only archive. Archiving cannot be undone
// (a new one can be created with the same parameter name). So destroy is implemented as archive.
// Note that changing `parameter_name` or `scope` archives the metric and creates it again under the new name.
type customMetricResource struct {
	svc *analyticsadmin.Service
}

type customMetricResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	Property             types.String `tfsdk:"property"`
	ParameterName        types.String `tfsdk:"parameter_name"`
	DisplayName          types.String `tfsdk:"display_name"`
	Description          types.String `tfsdk:"description"`
	MeasurementUnit      types.String `tfsdk:"measurement_unit"`
	Scope                types.String `tfsdk:"scope"`
	RestrictedMetricType types.Set    `tfsdk:"restricted_metric_type"`
}

func newCustomMetricResource() resource.Resource {
	return &customMetricResource{}
}

func (r *customMetricResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_metric"
}

func (r *customMetricResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A GA4 custom metric. GA4 has no delete, so destroy archives it. " +
			"Archiving cannot be undone (a new one can be created with the same `parameter_name`). " +
			"Changing `parameter_name` or `scope` archives the metric and creates it again under the new name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The custom metric's resource name (`properties/123/customMetrics/456`).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"property": schema.StringAttribute{
				Required:      true,
				Description:   "The resource name of the property it belongs to (`properties/123`). Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"parameter_name": schema.StringAttribute{
				Required: true,
				Description: "The event parameter name. Letters, digits and `_` only, starting with a letter, at most 40 characters. " +
					"Changing it forces replacement (the old one is archived).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"display_name": schema.StringAttribute{
				Required:    true,
				Description: "The display name shown in reports. At most 82 characters.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
				Description: "The description. At most 150 characters.",
			},
			"measurement_unit": schema.StringAttribute{
				Required: true,
				Description: "The unit of the value. One of `STANDARD`, `CURRENCY`, `FEET`, `METERS`, `KILOMETERS`, `MILES`, " +
					"`MILLISECONDS`, `SECONDS`, `MINUTES` or `HOURS`.",
				Validators: []validator.String{stringvalidator.OneOf(
					"STANDARD", "CURRENCY", "FEET", "METERS", "KILOMETERS", "MILES",
					"MILLISECONDS", "SECONDS", "MINUTES", "HOURS",
				)},
			},
			"scope": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString("EVENT"),
				Description:   "`EVENT` only. Changing it forces replacement.",
				Validators:    []validator.String{stringvalidator.OneOf("EVENT")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"restricted_metric_type": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				Description: "The kinds of restricted data this metric may contain: `COST_DATA` and `REVENUE_DATA`. " +
					"Required when `measurement_unit` is `CURRENCY`; leave it empty otherwise.",
				Validators: []validator.Set{setvalidator.ValueStringsAre(stringvalidator.OneOf("COST_DATA", "REVENUE_DATA"))},
			},
		},
	}
}

func (r *customMetricResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if svc := serviceFromResourceConfigure(req, &resp.Diagnostics); svc != nil {
		r.svc = svc
	}
}

func (r *customMetricResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customMetricResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	restricted, diags := stringsFromSet(ctx, plan.RestrictedMetricType)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.svc.Properties.CustomMetrics.Create(plan.Property.ValueString(), &analyticsadmin.GoogleAnalyticsAdminV1betaCustomMetric{
		ParameterName:        plan.ParameterName.ValueString(),
		DisplayName:          plan.DisplayName.ValueString(),
		Description:          plan.Description.ValueString(),
		MeasurementUnit:      plan.MeasurementUnit.ValueString(),
		Scope:                plan.Scope.ValueString(),
		RestrictedMetricType: restricted,
	}).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Unable to create GA4 custom metric", err.Error())
		return
	}

	model, err := customMetricModelFromAPI(created)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GA4 custom metric response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *customMetricResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customMetricResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.svc.Properties.CustomMetrics.Get(state.ID.ValueString()).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read GA4 custom metric", err.Error())
		return
	}

	model, err := customMetricModelFromAPI(got)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GA4 custom metric response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *customMetricResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state customMetricResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, mask, diags := customMetricPatch(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.svc.Properties.CustomMetrics.Patch(state.ID.ValueString(), body).UpdateMask(mask).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Unable to update GA4 custom metric", err.Error())
		return
	}

	model, err := customMetricModelFromAPI(updated)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GA4 custom metric response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// customMetricPatch builds the body and update_mask that Update sends. parameter_name and scope cannot change, so they are not sent.
// description and restricted_metric_type must be sent even when cleared, so ForceSendFields keeps them from being dropped
func customMetricPatch(ctx context.Context, plan customMetricResourceModel) (*analyticsadmin.GoogleAnalyticsAdminV1betaCustomMetric, string, diag.Diagnostics) {
	restricted, diags := stringsFromSet(ctx, plan.RestrictedMetricType)

	body := &analyticsadmin.GoogleAnalyticsAdminV1betaCustomMetric{
		DisplayName:          plan.DisplayName.ValueString(),
		Description:          plan.Description.ValueString(),
		MeasurementUnit:      plan.MeasurementUnit.ValueString(),
		RestrictedMetricType: restricted,
		ForceSendFields:      []string{"Description", "RestrictedMetricType"},
	}

	return body, "displayName,description,measurementUnit,restrictedMetricType", diags
}

// Delete archives the metric. GA4 has no delete for custom metrics
func (r *customMetricResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customMetricResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.svc.Properties.CustomMetrics.Archive(
		state.ID.ValueString(),
		&analyticsadmin.GoogleAnalyticsAdminV1betaArchiveCustomMetricRequest{},
	).Context(ctx).Do()
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Unable to archive GA4 custom metric", err.Error())
		return
	}
}

// ImportState accepts `terraform import ga4_custom_metric.x properties/123/customMetrics/456`
func (r *customMetricResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importResourceName(ctx, req, resp, "properties", "customMetrics")
}

func customMetricModelFromAPI(m *analyticsadmin.GoogleAnalyticsAdminV1betaCustomMetric) (*customMetricResourceModel, error) {
	property, err := parentOfName(m.Name)
	if err != nil {
		return nil, err
	}

	return &customMetricResourceModel{
		ID:                   types.StringValue(m.Name),
		Property:             types.StringValue(property),
		ParameterName:        types.StringValue(m.ParameterName),
		DisplayName:          types.StringValue(m.DisplayName),
		Description:          types.StringValue(m.Description),
		MeasurementUnit:      types.StringValue(m.MeasurementUnit),
		Scope:                types.StringValue(m.Scope),
		RestrictedMetricType: setFromStrings(m.RestrictedMetricType),
	}, nil
}

// stringsFromSet turns a set of strings into a slice to pass to the API. It returns nil for null or unknown
func stringsFromSet(ctx context.Context, s types.Set) ([]string, diag.Diagnostics) {
	if s.IsNull() || s.IsUnknown() {
		return nil, nil
	}

	var out []string
	diags := s.ElementsAs(ctx, &out, false)

	return out, diags
}

// setFromStrings turns a slice the API returned into a set of strings. The API omits the field when it is empty, so nil becomes an empty set
func setFromStrings(values []string) types.Set {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}

	return types.SetValueMust(types.StringType, elems)
}
