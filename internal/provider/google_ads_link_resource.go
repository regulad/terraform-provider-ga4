package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
)

var (
	_ resource.Resource                = (*googleAdsLinkResource)(nil)
	_ resource.ResourceWithConfigure   = (*googleAdsLinkResource)(nil)
	_ resource.ResourceWithImportState = (*googleAdsLinkResource)(nil)
)

// googleAdsLinkResource is a link between a GA4 property and a Google Ads account.
// v1beta has no get for a single link, so Read finds it by ID in the list.
type googleAdsLinkResource struct {
	svc *analyticsadmin.Service
}

type googleAdsLinkResourceModel struct {
	ID                        types.String `tfsdk:"id"`
	Property                  types.String `tfsdk:"property"`
	CustomerID                types.String `tfsdk:"customer_id"`
	AdsPersonalizationEnabled types.Bool   `tfsdk:"ads_personalization_enabled"`
	CanManageClients          types.Bool   `tfsdk:"can_manage_clients"`
	CreatorEmailAddress       types.String `tfsdk:"creator_email_address"`
}

func newGoogleAdsLinkResource() resource.Resource {
	return &googleAdsLinkResource{}
}

func (r *googleAdsLinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_google_ads_link"
}

func (r *googleAdsLinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A link between a GA4 property and a Google Ads account.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The link's resource name (`properties/123/googleAdsLinks/456`). The trailing ID is not the Google Ads customer ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"property": schema.StringAttribute{
				Required:      true,
				Description:   "The resource name of the property to link (`properties/123`). Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"customer_id": schema.StringAttribute{
				Required:      true,
				Description:   "The Google Ads customer ID (10 digits, without hyphens). Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ads_personalization_enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Description: "Whether to enable personalized advertising features (publishing audience lists and remarketing events to Google Ads). " +
					"If omitted, it takes GA4's default (true) on creation, and keeps the value in GA4 after that.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"can_manage_clients": schema.BoolAttribute{
				Computed:      true,
				Description:   "True if the linked account is a Google Ads manager (MCC) account.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"creator_email_address": schema.StringAttribute{
				Computed:      true,
				Description:   "The email address of the user who created the link, or empty if it is not available.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *googleAdsLinkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if svc := serviceFromResourceConfigure(req, &resp.Diagnostics); svc != nil {
		r.svc = svc
	}
}

func (r *googleAdsLinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan googleAdsLinkResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := &analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink{
		CustomerId: plan.CustomerID.ValueString(),
	}
	// If it is not set, don't send it, and leave it to GA4's default (true)
	if !plan.AdsPersonalizationEnabled.IsNull() && !plan.AdsPersonalizationEnabled.IsUnknown() {
		body.AdsPersonalizationEnabled = plan.AdsPersonalizationEnabled.ValueBool()
		body.ForceSendFields = []string{"AdsPersonalizationEnabled"}
	}

	created, err := r.svc.Properties.GoogleAdsLinks.Create(plan.Property.ValueString(), body).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Unable to create Google Ads link", err.Error())
		return
	}

	model, err := googleAdsLinkModelFromAPI(created)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Google Ads link response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *googleAdsLinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state googleAdsLinkResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.findGoogleAdsLink(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Google Ads link", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	model, err := googleAdsLinkModelFromAPI(got)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Google Ads link response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// findGoogleAdsLink finds the link with the given name in the property's list of links. It returns nil if there is none.
// If the property itself is gone, the list returns 404
func (r *googleAdsLinkResource) findGoogleAdsLink(ctx context.Context, name string) (*analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink, error) {
	property, err := parentOfName(name)
	if err != nil {
		return nil, err
	}

	var found *analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink
	err = r.svc.Properties.GoogleAdsLinks.List(property).Pages(ctx, func(page *analyticsadmin.GoogleAnalyticsAdminV1betaListGoogleAdsLinksResponse) error {
		if found == nil {
			found = findGoogleAdsLinkByName(page.GoogleAdsLinks, name)
		}
		return nil
	})

	return found, err
}

func findGoogleAdsLinkByName(links []*analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink, name string) *analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink {
	for _, link := range links {
		if link.Name == name {
			return link
		}
	}

	return nil
}

func (r *googleAdsLinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state googleAdsLinkResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only ads_personalization_enabled can change. false must be sent too, so ForceSendFields keeps it from being dropped
	updated, err := r.svc.Properties.GoogleAdsLinks.Patch(state.ID.ValueString(), &analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink{
		AdsPersonalizationEnabled: plan.AdsPersonalizationEnabled.ValueBool(),
		ForceSendFields:           []string{"AdsPersonalizationEnabled"},
	}).UpdateMask("adsPersonalizationEnabled").Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Unable to update Google Ads link", err.Error())
		return
	}

	model, err := googleAdsLinkModelFromAPI(updated)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Google Ads link response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *googleAdsLinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state googleAdsLinkResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.svc.Properties.GoogleAdsLinks.Delete(state.ID.ValueString()).Context(ctx).Do(); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete Google Ads link", err.Error())
		return
	}
}

// ImportState accepts `terraform import ga4_google_ads_link.x properties/123/googleAdsLinks/456`
func (r *googleAdsLinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importResourceName(ctx, req, resp, "properties", "googleAdsLinks")
}

func googleAdsLinkModelFromAPI(l *analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink) (*googleAdsLinkResourceModel, error) {
	property, err := parentOfName(l.Name)
	if err != nil {
		return nil, err
	}

	return &googleAdsLinkResourceModel{
		ID:                        types.StringValue(l.Name),
		Property:                  types.StringValue(property),
		CustomerID:                types.StringValue(l.CustomerId),
		AdsPersonalizationEnabled: types.BoolValue(l.AdsPersonalizationEnabled),
		CanManageClients:          types.BoolValue(l.CanManageClients),
		CreatorEmailAddress:       types.StringValue(l.CreatorEmailAddress),
	}, nil
}
