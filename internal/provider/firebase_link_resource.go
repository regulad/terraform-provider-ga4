package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
)

var (
	_ resource.Resource                = (*firebaseLinkResource)(nil)
	_ resource.ResourceWithConfigure   = (*firebaseLinkResource)(nil)
	_ resource.ResourceWithImportState = (*firebaseLinkResource)(nil)
)

// firebaseLinkResource is a link between a GA4 property and a Firebase project. A property has at most one.
//
// The API has no update, so changing any attribute forces replacement. v1beta has no get for a single link either, so
// Read finds it by ID in the list.
type firebaseLinkResource struct {
	svc *analyticsadmin.Service
}

type firebaseLinkResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Property types.String `tfsdk:"property"`
	Project  types.String `tfsdk:"project"`
}

// firebaseProjectNumberPattern is the form of a project number (`projects/1234`). The API returns only this form
var firebaseProjectNumberPattern = regexp.MustCompile(`^projects/[0-9]+$`)

func newFirebaseLinkResource() resource.Resource {
	return &firebaseLinkResource{}
}

func (r *firebaseLinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firebase_link"
}

func (r *firebaseLinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A link between a GA4 property and a Firebase project. A property has at most one. There is no update: changing any attribute forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The link's resource name (`properties/123/firebaseLinks/456`).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"property": schema.StringAttribute{
				Required:      true,
				Description:   "The resource name of the property to link (`properties/123`). Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"project": schema.StringAttribute{
				Required: true,
				Description: "The Firebase project's resource name: `projects/<project number>` or `projects/<project ID>`. " +
					"The API returns only the number form, so a value written in the ID form is kept as written. Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *firebaseLinkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if svc := serviceFromResourceConfigure(req, &resp.Diagnostics); svc != nil {
		r.svc = svc
	}
}

func (r *firebaseLinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firebaseLinkResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.svc.Properties.FirebaseLinks.Create(plan.Property.ValueString(), &analyticsadmin.GoogleAnalyticsAdminV1betaFirebaseLink{
		Project: plan.Project.ValueString(),
	}).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Unable to create Firebase link", err.Error())
		return
	}

	model, err := firebaseLinkModelFromAPI(created, plan.Project.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Firebase link response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *firebaseLinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firebaseLinkResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.findFirebaseLink(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Firebase link", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// Right after an import, project is null, so it takes the API's value (in the number form)
	model, err := firebaseLinkModelFromAPI(got, state.Project.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Firebase link response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// findFirebaseLink finds the link with the given name in the property's list of links. It returns nil if there is none.
// If the property itself is gone, the list returns 404
func (r *firebaseLinkResource) findFirebaseLink(ctx context.Context, name string) (*analyticsadmin.GoogleAnalyticsAdminV1betaFirebaseLink, error) {
	property, err := parentOfName(name)
	if err != nil {
		return nil, err
	}

	var found *analyticsadmin.GoogleAnalyticsAdminV1betaFirebaseLink
	err = r.svc.Properties.FirebaseLinks.List(property).Pages(ctx, func(page *analyticsadmin.GoogleAnalyticsAdminV1betaListFirebaseLinksResponse) error {
		for _, link := range page.FirebaseLinks {
			if found == nil && link.Name == name {
				found = link
			}
		}
		return nil
	})

	return found, err
}

// Update is never called. Changing any attribute forces replacement
func (r *firebaseLinkResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Provider internal error", "Firebase links cannot be updated, so Update should never be called.")
}

func (r *firebaseLinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firebaseLinkResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.svc.Properties.FirebaseLinks.Delete(state.ID.ValueString()).Context(ctx).Do(); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete Firebase link", err.Error())
		return
	}
}

// ImportState accepts `terraform import ga4_firebase_link.x properties/123/firebaseLinks/456`
func (r *firebaseLinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importResourceName(ctx, req, resp, "properties", "firebaseLinks")
}

// firebaseLinkModelFromAPI turns the API's response into state. known is the project value from the configuration or state.
// The API always returns project in the number form, so if known is in the ID form, the written value is kept (this API cannot confirm which number an ID maps to).
// If known is in the number form and differs from the API's value, it was relinked elsewhere, so the API's value is used
func firebaseLinkModelFromAPI(l *analyticsadmin.GoogleAnalyticsAdminV1betaFirebaseLink, known string) (*firebaseLinkResourceModel, error) {
	property, err := parentOfName(l.Name)
	if err != nil {
		return nil, err
	}

	project := l.Project
	if known != "" && !firebaseProjectNumberPattern.MatchString(known) {
		project = known
	}

	return &firebaseLinkResourceModel{
		ID:       types.StringValue(l.Name),
		Property: types.StringValue(property),
		Project:  types.StringValue(project),
	}, nil
}
