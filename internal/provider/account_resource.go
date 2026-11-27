package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
)

var (
	_ resource.Resource                = (*accountResource)(nil)
	_ resource.ResourceWithConfigure   = (*accountResource)(nil)
	_ resource.ResourceWithImportState = (*accountResource)(nil)
)

// accountResource brings an existing GA4 account under Terraform's management.
//
// Accounts cannot be created through the API. v1beta has only provisionAccountTicket,
// which becomes an account only once a person accepts the terms of service in the admin UI. So create
// fails, and an account made in the admin UI is imported instead.
//
// destroy calls the API's delete, but deleting a GA4 account moves it to the trash,
// from which it can be restored in the admin UI for 35 days. If it is not restored, it is deleted for good, with its properties and streams.
type accountResource struct {
	svc *analyticsadmin.Service
}

type accountResourceModel struct {
	ID              types.String `tfsdk:"id"`
	DisplayName     types.String `tfsdk:"display_name"`
	RegionCode      types.String `tfsdk:"region_code"`
	GmpOrganization types.String `tfsdk:"gmp_organization"`
}

func newAccountResource() resource.Resource {
	return &accountResource{}
}

func (r *accountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account"
}

func (r *accountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An existing GA4 account. Accounts cannot be created through the API, so import one made in the admin UI (an apply without an import fails). " +
			"destroy moves the account to the trash, from which it can be restored in the admin UI for 35 days. If it is not restored, it is deleted for good, with its properties.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The account's resource name (`accounts/123`). Pass it as `ga4_property`'s `account`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"display_name": schema.StringAttribute{
				Required:    true,
				Description: "The account's display name.",
			},
			"region_code": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "The country of the business (a Unicode CLDR region code, e.g. `JP`). If omitted, the value in GA4 is kept.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"gmp_organization": schema.StringAttribute{
				Computed:      true,
				Description:   "The URI of the Google Marketing Platform organization the account is linked to, or empty if it is not linked to one.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *accountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if svc := serviceFromResourceConfigure(req, &resp.Diagnostics); svc != nil {
		r.svc = svc
	}
}

// Create fails: accounts cannot be created through the API
func (r *accountResource) Create(_ context.Context, _ resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError(
		"GA4 accounts cannot be created through the API",
		"Create the account in the GA4 admin UI, import it with `terraform import ga4_account.<name> accounts/<account ID>` "+
			"(or an import block), then apply. "+
			"To refer to an existing account without managing it, use `data.ga4_account`.",
	)
}

func (r *accountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state accountResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.svc.Accounts.Get(state.ID.ValueString()).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read GA4 account", err.Error())
		return
	}

	// Get can still return an account in the trash, but it can no longer be used, so treat it as gone
	if got.Deleted {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, accountModelFromAPI(got))...)
}

func (r *accountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state accountResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, mask := accountPatch(plan)

	updated, err := r.svc.Accounts.Patch(state.ID.ValueString(), body).UpdateMask(mask).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Unable to update GA4 account", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, accountModelFromAPI(updated))...)
}

// accountPatch builds the body and update_mask that Update sends.
// region_code is optional, so when it is not set, the value in GA4 is kept
func accountPatch(plan accountResourceModel) (*analyticsadmin.GoogleAnalyticsAdminV1betaAccount, string) {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaAccount{
		DisplayName: plan.DisplayName.ValueString(),
	}
	mask := []string{"displayName"}

	if v := knownString(plan.RegionCode); v != "" {
		body.RegionCode = v
		mask = append(mask, "regionCode")
	}

	return body, strings.Join(mask, ",")
}

func (r *accountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state accountResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.svc.Accounts.Delete(state.ID.ValueString()).Context(ctx).Do(); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete GA4 account (move it to the trash)", err.Error())
		return
	}
}

// ImportState accepts `terraform import ga4_account.x accounts/123`
func (r *accountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importResourceName(ctx, req, resp, "accounts")
}

func accountModelFromAPI(a *analyticsadmin.GoogleAnalyticsAdminV1betaAccount) *accountResourceModel {
	return &accountResourceModel{
		ID:              types.StringValue(a.Name),
		DisplayName:     types.StringValue(a.DisplayName),
		RegionCode:      types.StringValue(a.RegionCode),
		GmpOrganization: types.StringValue(a.GmpOrganization),
	}
}
