package azuread

import (
	"context"

	"github.com/manicminer/hamilton/msgraph"
	"github.com/manicminer/hamilton/odata"
	"github.com/turbot/steampipe-plugin-sdk/grpc/proto"
	"github.com/turbot/steampipe-plugin-sdk/plugin/transform"

	"github.com/turbot/steampipe-plugin-sdk/plugin"
)

//// TABLE DEFINITION

func tableAzureAdApplication() *plugin.Table {
	return &plugin.Table{
		Name:        "azuread_application",
		Description: "Represents an Azure Active Directory (Azure AD) application",
		Get: &plugin.GetConfig{
			Hydrate:           getAdApplication,
			KeyColumns:        plugin.SingleColumn("id"),
			ShouldIgnoreError: isNotFoundError,
		},
		List: &plugin.ListConfig{
			Hydrate: listAdApplications,
			KeyColumns: plugin.KeyColumnSlice{
				// Key fields
				{Name: "id", Require: plugin.Optional},
				{Name: "display_name", Require: plugin.Optional},
				{Name: "filter", Require: plugin.Optional},

				// Other fields for filtering OData
				// {Name: "mail", Require: plugin.Optional, Operators: []string{"<>", "="}},                     // $filter (eq, ne, NOT, ge, le, in, startsWith).
				// {Name: "mail_enabled", Require: plugin.Optional, Operators: []string{"<>", "="}},             // $filter (eq, ne, NOT).
				// {Name: "on_premises_sync_enabled", Require: plugin.Optional, Operators: []string{"<>", "="}}, // $filter (eq, ne, NOT, in).
				// {Name: "security_enabled", Require: plugin.Optional, Operators: []string{"<>", "="}},         // $filter (eq, ne, NOT, in).

				// TODO
				// {Name: "created_date_time", Operators: []string{">", ">=", "=", "<", "<="}, Require: plugin.Optional},    // Supports $filter (eq, ne, NOT, ge, le, in).
				// {Name: "expiration_date_time", Operators: []string{">", ">=", "=", "<", "<="}, Require: plugin.Optional}, // Supports $filter (eq, ne, NOT, ge, le, in).
			},
		},

		Columns: []*plugin.Column{
			{Name: "display_name", Type: proto.ColumnType_STRING, Description: "The display name for the application."},
			{Name: "id", Type: proto.ColumnType_STRING, Description: "The unique identifier for the application.", Transform: transform.FromGo()},
			{Name: "app_id", Type: proto.ColumnType_STRING, Description: "The unique identifier for the application that is assigned to an application by Azure AD."},
			{Name: "filter", Type: proto.ColumnType_STRING, Transform: transform.FromQual("filter"), Description: "Odata query to search for applications."},

			// // Other fields
			{Name: "sign_in_audience", Type: proto.ColumnType_STRING, Description: "Specifies the Microsoft accounts that are supported for the current application."},
			{Name: "created_date_time", Type: proto.ColumnType_TIMESTAMP, Description: "The date and time the application was registered. The DateTimeOffset type represents date and time information using ISO 8601 format and is always in UTC time."},
			{Name: "is_authorization_service_enabled", Type: proto.ColumnType_BOOL, Description: "Is authorization service enabled."},
			{Name: "oauth2_require_post_response", Type: proto.ColumnType_BOOL, Description: "Specifies whether, as part of OAuth 2.0 token requests, Azure AD allows POST requests, as opposed to GET requests. The default is false, which specifies that only GET requests are allowed."},
			{Name: "publisher_domain", Type: proto.ColumnType_STRING, Description: "The verified publisher domain for the application."},

			// // Json fields
			{Name: "api", Type: proto.ColumnType_JSON, Description: "Specifies settings for an application that implements a web API."},
			{Name: "spa", Type: proto.ColumnType_JSON, Description: "Specifies settings for a single-page application, including sign out URLs and redirect URIs for authorization codes and access tokens."},
			{Name: "web", Type: proto.ColumnType_JSON, Description: "Specifies settings for a web application."},
			{Name: "owner_ids", Type: proto.ColumnType_JSON, Hydrate: getApplicationOwners, Transform: transform.FromValue(), Description: "Id of the owners of the application. The owners are a set of non-admin users who are allowed to modify this object."},
			{Name: "info", Type: proto.ColumnType_JSON, Description: "Basic profile information of the application such as app's marketing, support, terms of service and privacy statement URLs. The terms of service and privacy statement are surfaced to users through the user consent experience."},
			{Name: "identifier_uris", Type: proto.ColumnType_JSON, Description: "The URIs that identify the application within its Azure AD tenant, or within a verified custom domain if the application is multi-tenant. "},
			{Name: "parental_control_settings", Type: proto.ColumnType_JSON, Description: "Specifies parental control settings for an application."},
			{Name: "password_credentials", Type: proto.ColumnType_JSON, Description: "The collection of password credentials associated with the application."},
			{Name: "key_credentials", Type: proto.ColumnType_JSON, Description: "The collection of key credentials associated with the application."},
			{Name: "tags", Type: proto.ColumnType_JSON, Description: "Custom strings that can be used to categorize and identify the application."},

			// // Standard columns
			// {Name: "tags", Type: proto.ColumnType_STRING, Description: ColumnDescriptionTags, Transform: transform.From(applicationTags)},
			{Name: "title", Type: proto.ColumnType_STRING, Description: ColumnDescriptionTitle, Transform: transform.FromField("DisplayName", "ID")},
			{Name: "tenant_id", Type: proto.ColumnType_STRING, Description: ColumnDescriptionTenant, Hydrate: plugin.HydrateFunc(getTenantId).WithCache(), Transform: transform.FromValue()},
			// {Name: "data", Type: proto.ColumnType_JSON, Description: "The unique ID that identifies an active directory user.", Transform: transform.FromValue()}, // For debugging
		},
	}
}

//// LIST FUNCTION

func listAdApplications(ctx context.Context, d *plugin.QueryData, _ *plugin.HydrateData) (interface{}, error) {
	session, err := GetNewSession(ctx, d)
	if err != nil {
		return nil, err
	}

	client := msgraph.NewApplicationsClient(session.TenantID)
	client.BaseClient.Authorizer = session.Authorizer

	// TODO filters
	input := odata.Query{}
	filter := ""
	input.Filter = filter

	// if input.Filter != "" {
	// 	plugin.Logger(ctx).Debug("Filter", "input.Filter", input.Filter)
	// }

	pagesLeft := true
	for pagesLeft {
		applications, _, err := client.List(ctx, input)
		if err != nil {
			if isNotFoundError(err) {
				return nil, nil
			}
			return nil, err
		}

		for _, application := range *applications {
			d.StreamListItem(ctx, application)
		}
		pagesLeft = false
	}

	return nil, err
}

// Hydrate Functions

func getAdApplication(ctx context.Context, d *plugin.QueryData, h *plugin.HydrateData) (interface{}, error) {
	session, err := GetNewSession(ctx, d)
	if err != nil {
		return nil, err
	}

	client := msgraph.NewApplicationsClient(session.TenantID)
	client.BaseClient.Authorizer = session.Authorizer

	var applicationId string
	if h.Item != nil {
		applicationId = *h.Item.(msgraph.ServicePrincipal).ID
	} else {
		applicationId = d.KeyColumnQuals["id"].GetStringValue()
	}

	// TODO filters
	input := odata.Query{}
	filter := ""
	input.Filter = filter

	applications, _, err := client.Get(ctx, applicationId, input)
	if err != nil {
		return nil, err
	}
	return *applications, nil
}

func getApplicationOwners(ctx context.Context, d *plugin.QueryData, h *plugin.HydrateData) (interface{}, error) {
	application := h.Item.(msgraph.Application)
	session, err := GetNewSession(ctx, d)
	if err != nil {
		return nil, err
	}

	client := msgraph.NewApplicationsClient(session.TenantID)
	client.BaseClient.Authorizer = session.Authorizer

	owners, _, err := client.ListOwners(ctx, *application.ID)
	if err != nil {
		return nil, err
	}
	return owners, nil
}