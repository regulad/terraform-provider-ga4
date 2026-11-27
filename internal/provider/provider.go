// Package provider は GA4 Admin API を Terraform から扱うためのプロバイダ実装。
//
// It covers what Admin API v1beta offers: accounts (by importing existing ones), properties, data retention settings,
// data streams (web / Android / iOS), Measurement Protocol API secrets, custom dimensions,
// custom metrics, key events, Google Ads links and Firebase links.
// Settings not in v1beta, such as reporting identity, are not supported; set them in the GA4 admin UI.
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*ga4Provider)(nil)

type ga4Provider struct {
	version string
}

type ga4ProviderModel struct {
	Credentials types.String `tfsdk:"credentials"`
}

// New はプロバイダのファクトリを返す。version はリリースのバージョン文字列
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ga4Provider{version: version}
	}
}

func (p *ga4Provider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ga4"
	resp.Version = p.version
}

func (p *ga4Provider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages, through the Google Analytics 4 Admin API, accounts, properties, data retention settings, data streams, Measurement Protocol API secrets, " +
			"custom dimensions, custom metrics, key events, and Google Ads and Firebase links.",
		Attributes: map[string]schema.Attribute{
			"credentials": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "サービスアカウントの JSON キー。ファイルパスか JSON の中身そのもの。" +
					"省略すると Application Default Credentials（`GOOGLE_APPLICATION_CREDENTIALS` または `gcloud auth application-default login`）を使う。" +
					"いずれの場合も、その資格情報の主体を GA4 アカウントかプロパティに「編集者」として追加しておくこと。",
			},
		},
	}
}

func (p *ga4Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config ga4ProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	svc, err := newAdminService(ctx, config.Credentials.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("GA4 Admin API に接続できません", err.Error())
		return
	}

	resp.DataSourceData = svc
	resp.ResourceData = svc
}

func (p *ga4Provider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newPropertyResource,
		newDataStreamResource,
		newMeasurementProtocolSecretResource,
		newCustomDimensionResource,
		newAccountResource,
		newDataRetentionSettingsResource,
		newCustomMetricResource,
		newKeyEventResource,
		newGoogleAdsLinkResource,
		newFirebaseLinkResource,
	}
}

func (p *ga4Provider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newAccountDataSource,
	}
}
