package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
)

var (
	_ resource.Resource                = (*keyEventResource)(nil)
	_ resource.ResourceWithConfigure   = (*keyEventResource)(nil)
	_ resource.ResourceWithImportState = (*keyEventResource)(nil)
)

// keyEventResource is a GA4 key event (formerly a "conversion").
// v1beta's conversionEvents is deprecated, so it is not used; only its successor, keyEvents, is.
type keyEventResource struct {
	svc *analyticsadmin.Service
}

type keyEventResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Property       types.String `tfsdk:"property"`
	EventName      types.String `tfsdk:"event_name"`
	CountingMethod types.String `tfsdk:"counting_method"`
	DefaultValue   types.Object `tfsdk:"default_value"`
	Custom         types.Bool   `tfsdk:"custom"`
	Deletable      types.Bool   `tfsdk:"deletable"`
}

type keyEventDefaultValueModel struct {
	NumericValue types.Float64 `tfsdk:"numeric_value"`
	CurrencyCode types.String  `tfsdk:"currency_code"`
}

var keyEventDefaultValueAttrTypes = map[string]attr.Type{
	"numeric_value": types.Float64Type,
	"currency_code": types.StringType,
}

func newKeyEventResource() resource.Resource {
	return &keyEventResource{}
}

func (r *keyEventResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_key_event"
}

func (r *keyEventResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A GA4 key event (formerly a \"conversion\"). Events with the given event name are counted as key events.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The key event's resource name (`properties/123/keyEvents/456`).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"property": schema.StringAttribute{
				Required:      true,
				Description:   "The resource name of the property it belongs to (`properties/123`). Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"event_name": schema.StringAttribute{
				Required:      true,
				Description:   "The name of the event to make a key event (e.g. `purchase`). Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"counting_method": schema.StringAttribute{
				Required: true,
				Description: "How it is counted: `ONCE_PER_EVENT` (counted for every event) or " +
					"`ONCE_PER_SESSION` (at most once per session).",
				Validators: []validator.String{stringvalidator.OneOf("ONCE_PER_EVENT", "ONCE_PER_SESSION")},
			},
			"default_value": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "The default value used when an event has no `value` or `currency` parameter. If omitted, there is no default.",
				Attributes: map[string]schema.Attribute{
					"numeric_value": schema.Float64Attribute{
						Required:    true,
						Description: "The value used when there is no `value` parameter.",
					},
					"currency_code": schema.StringAttribute{
						Required:    true,
						Description: "The currency used when there is no `currency` parameter (ISO 4217, e.g. `JPY`).",
					},
				},
			},
			"custom": schema.BoolAttribute{
				Computed:      true,
				Description:   "True for a key event on a custom event, false for one on a GA4 default event (such as `purchase`).",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"deletable": schema.BoolAttribute{
				Computed:      true,
				Description:   "Whether it can be deleted. destroy fails on one where this is false.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *keyEventResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if svc := serviceFromResourceConfigure(req, &resp.Diagnostics); svc != nil {
		r.svc = svc
	}
}

func (r *keyEventResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan keyEventResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	defaultValue, diags := keyEventDefaultValueFromPlan(ctx, plan.DefaultValue)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.svc.Properties.KeyEvents.Create(plan.Property.ValueString(), &analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent{
		EventName:      plan.EventName.ValueString(),
		CountingMethod: plan.CountingMethod.ValueString(),
		DefaultValue:   defaultValue,
	}).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Unable to create GA4 key event", err.Error())
		return
	}

	model, err := keyEventModelFromAPI(created)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GA4 key event response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *keyEventResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state keyEventResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.svc.Properties.KeyEvents.Get(state.ID.ValueString()).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read GA4 key event", err.Error())
		return
	}

	model, err := keyEventModelFromAPI(got)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GA4 key event response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *keyEventResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state keyEventResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, mask, diags := keyEventPatch(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.svc.Properties.KeyEvents.Patch(state.ID.ValueString(), body).UpdateMask(mask).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Unable to update GA4 key event", err.Error())
		return
	}

	model, err := keyEventModelFromAPI(updated)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GA4 key event response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// keyEventPatch builds the body and update_mask that Update sends. event_name cannot change, so it is not sent.
// When default_value is removed, it goes in update_mask only, not in the body, which clears the default
func keyEventPatch(ctx context.Context, plan keyEventResourceModel) (*analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent, string, diag.Diagnostics) {
	defaultValue, diags := keyEventDefaultValueFromPlan(ctx, plan.DefaultValue)

	body := &analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent{
		CountingMethod: plan.CountingMethod.ValueString(),
		DefaultValue:   defaultValue,
	}

	return body, "countingMethod,defaultValue", diags
}

func (r *keyEventResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state keyEventResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.svc.Properties.KeyEvents.Delete(state.ID.ValueString()).Context(ctx).Do(); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete GA4 key event", err.Error())
		return
	}
}

// ImportState accepts `terraform import ga4_key_event.x properties/123/keyEvents/456`
func (r *keyEventResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importResourceName(ctx, req, resp, "properties", "keyEvents")
}

// keyEventDefaultValueFromPlan converts the plan's default_value to the API's type. It returns nil for null or unknown
func keyEventDefaultValueFromPlan(ctx context.Context, v types.Object) (*analyticsadmin.GoogleAnalyticsAdminV1betaKeyEventDefaultValue, diag.Diagnostics) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}

	var m keyEventDefaultValueModel
	diags := v.As(ctx, &m, basetypes.ObjectAsOptions{})
	if diags.HasError() {
		return nil, diags
	}

	return &analyticsadmin.GoogleAnalyticsAdminV1betaKeyEventDefaultValue{
		NumericValue: m.NumericValue.ValueFloat64(),
		CurrencyCode: m.CurrencyCode.ValueString(),
		// 0 must be sent as a default too, so ForceSendFields keeps it from being dropped
		ForceSendFields: []string{"NumericValue"},
	}, diags
}

func keyEventModelFromAPI(e *analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent) (*keyEventResourceModel, error) {
	property, err := parentOfName(e.Name)
	if err != nil {
		return nil, err
	}

	defaultValue := types.ObjectNull(keyEventDefaultValueAttrTypes)
	if e.DefaultValue != nil {
		defaultValue = types.ObjectValueMust(keyEventDefaultValueAttrTypes, map[string]attr.Value{
			"numeric_value": types.Float64Value(e.DefaultValue.NumericValue),
			"currency_code": types.StringValue(e.DefaultValue.CurrencyCode),
		})
	}

	return &keyEventResourceModel{
		ID:             types.StringValue(e.Name),
		Property:       types.StringValue(property),
		EventName:      types.StringValue(e.EventName),
		CountingMethod: types.StringValue(e.CountingMethod),
		DefaultValue:   defaultValue,
		Custom:         types.BoolValue(e.Custom),
		Deletable:      types.BoolValue(e.Deletable),
	}, nil
}
