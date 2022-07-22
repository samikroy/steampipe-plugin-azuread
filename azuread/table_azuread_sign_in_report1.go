package azuread

import (
	"context"
	"errors"
	"fmt"

	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/auditlogs/signins"
	"github.com/microsoftgraph/msgraph-sdk-go/models"

	"github.com/turbot/steampipe-plugin-sdk/v3/grpc/proto"
	"github.com/turbot/steampipe-plugin-sdk/v3/plugin"
	"github.com/turbot/steampipe-plugin-sdk/v3/plugin/transform"
)

//// TABLE DEFINITION

func tableAzureAdSignInReportTest() *plugin.Table {
	return &plugin.Table{
		Name:        "azuread_sign_in_report_test",
		Description: "Represents an Azure Active Directory (Azure AD) sign in report",
		Get: &plugin.GetConfig{
			Hydrate:    getAdSignInReportTest,
			KeyColumns: plugin.SingleColumn("id"),
		},
		List: &plugin.ListConfig{
			Hydrate: listAdSignInReportsTest,
		},

		Columns: []*plugin.Column{
			{Name: "id", Type: proto.ColumnType_STRING, Description: "Unique ID representing the sign-in activity.", Transform: transform.FromMethod("GetId")},
			{Name: "created_date_time", Type: proto.ColumnType_TIMESTAMP, Description: "Date and time (UTC) the sign-in was initiated.", Transform: transform.FromMethod("GetCreatedDateTime")},
			{Name: "user_display_name", Type: proto.ColumnType_STRING, Description: "Display name of the user that initiated the sign-in.", Transform: transform.FromMethod("GetUserDisplayName")},
			{Name: "user_principal_name", Type: proto.ColumnType_STRING, Description: "User principal name of the user that initiated the sign-in.", Transform: transform.FromMethod("GetUserPrincipalName")},
			{Name: "user_id", Type: proto.ColumnType_STRING, Description: "ID of the user that initiated the sign-in.", Transform: transform.FromMethod("GetUserId")},
			{Name: "app_id", Type: proto.ColumnType_STRING, Description: "Unique GUID representing the app ID in the Azure Active Directory.", Transform: transform.FromMethod("GetAppId")},
			{Name: "app_display_name", Type: proto.ColumnType_STRING, Description: "App name displayed in the Azure Portal.", Transform: transform.FromMethod("GetAppDisplayName")},
			{Name: "ip_address", Type: proto.ColumnType_IPADDR, Description: "IP address of the client used to sign in.", Transform: transform.FromMethod("GetIpAddress")},
			{Name: "client_app_used", Type: proto.ColumnType_STRING, Description: "Identifies the legacy client used for sign-in activity.", Transform: transform.FromMethod("GetClientAppUsed")},
			{Name: "correlation_id", Type: proto.ColumnType_STRING, Description: "The request ID sent from the client when the sign-in is initiated; used to troubleshoot sign-in activity.", Transform: transform.FromMethod("GetCorrelationId")},
			{Name: "conditional_access_status", Type: proto.ColumnType_STRING, Description: "Reports status of an activated conditional access policy. Possible values are: success, failure, notApplied, and unknownFutureValue.", Transform: transform.FromMethod("GetConditionalAccessStatus")},
			{Name: "is_interactive", Type: proto.ColumnType_BOOL, Description: "Indicates if a sign-in is interactive or not.", Transform: transform.FromMethod("GetIsInteractive")},
			{Name: "risk_detail", Type: proto.ColumnType_STRING, Description: "Provides the 'reason' behind a specific state of a risky user, sign-in or a risk event. The possible values are: none, adminGeneratedTemporaryPassword, userPerformedSecuredPasswordChange, userPerformedSecuredPasswordReset, adminConfirmedSigninSafe, aiConfirmedSigninSafe, userPassedMFADrivenByRiskBasedPolicy, adminDismissedAllRiskForUser, adminConfirmedSigninCompromised, unknownFutureValue.", Transform: transform.FromMethod("GetRiskDetail")},
			{Name: "risk_level_aggregated", Type: proto.ColumnType_STRING, Description: "Aggregated risk level. The possible values are: none, low, medium, high, hidden, and unknownFutureValue.", Transform: transform.FromMethod("GetRiskLevelAggregated")},
			{Name: "risk_level_during_sign_in", Type: proto.ColumnType_STRING, Description: "Risk level during sign-in. The possible values are: none, low, medium, high, hidden, and unknownFutureValue.", Transform: transform.FromMethod("GetRiskLevelDuringSignIn")},
			{Name: "risk_state", Type: proto.ColumnType_STRING, Description: "Reports status of the risky user, sign-in, or a risk event. The possible values are: none, confirmedSafe, remediated, dismissed, atRisk, confirmedCompromised, unknownFutureValue.", Transform: transform.FromMethod("GetRiskState")},
			{Name: "resource_display_name", Type: proto.ColumnType_STRING, Description: "Name of the resource the user signed into.", Transform: transform.FromMethod("GetResourceDisplayName")},
			{Name: "resource_id", Type: proto.ColumnType_STRING, Description: "ID of the resource that the user signed into.", Transform: transform.FromMethod("GetResourceId")},

			// Json fields
			{Name: "risk_event_types", Type: proto.ColumnType_JSON, Description: "Risk event types associated with the sign-in. The possible values are: unlikelyTravel, anonymizedIPAddress, maliciousIPAddress, unfamiliarFeatures, malwareInfectedIPAddress, suspiciousIPAddress, leakedCredentials, investigationsThreatIntelligence, generic, and unknownFutureValue.", Transform: transform.FromMethod("GetRiskEventTypes")},
			{Name: "status", Type: proto.ColumnType_JSON, Description: "Sign-in status. Includes the error code and description of the error (in case of a sign-in failure).", Transform: transform.FromMethod("SignInStatus")},
			{Name: "device_detail", Type: proto.ColumnType_JSON, Description: "Device information from where the sign-in occurred; includes device ID, operating system, and browser.", Transform: transform.FromMethod("SignInDeviceDetail")},
			{Name: "location", Type: proto.ColumnType_JSON, Description: "	Provides the city, state, and country code where the sign-in originated.", Transform: transform.FromMethod("SignInLocation")},
			{Name: "applied_conditional_access_policies", Type: proto.ColumnType_JSON, Description: "Provides a list of conditional access policies that are triggered by the corresponding sign-in activity.", Transform: transform.FromMethod("SignInAppliedConditionalAccessPolicies")},

			// Standard columns
			{Name: "title", Type: proto.ColumnType_STRING, Description: ColumnDescriptionTitle, Transform: transform.FromMethod("GetId")},
			{Name: "tenant_id", Type: proto.ColumnType_STRING, Description: ColumnDescriptionTenant, Hydrate: plugin.HydrateFunc(getTenant).WithCache(), Transform: transform.FromValue()},
		},
	}
}

//// LIST FUNCTION

func listAdSignInReportsTest(ctx context.Context, d *plugin.QueryData, _ *plugin.HydrateData) (interface{}, error) {
	// Create client
	client, adapter, err := GetGraphClient(ctx, d)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("error creating client: %v", err))
	}

	// List operations
	input := &signins.SignInsRequestBuilderGetQueryParameters{}

	// Restrict the limit value to be passed in the query parameter which is not between 1 and 999, otherwise API will throw an error as follow
	// unexpected status 400 with OData error: Request_UnsupportedQuery: Invalid page size specified: '1000'. Must be between 1 and 999 inclusive.
	limit := d.QueryContext.Limit
	if limit != nil {
		if *limit > 0 && *limit <= 999 {
			l := int32(*limit)
			input.Top = &l
		}
	}

	options := &signins.SignInsRequestBuilderGetRequestConfiguration{
		QueryParameters: input,
	}

	result, err := client.AuditLogs().SignIns().GetWithRequestConfigurationAndResponseHandler(options, nil)
	if err != nil {
		errObj := getErrorObject(err)
		return nil, errors.New(fmt.Sprintf("failed to list groups. Code: %s Message: %s", errObj.Code, errObj.Message))
	}

	pageIterator, err := msgraphcore.NewPageIterator(result, adapter, models.CreateSignInCollectionResponseFromDiscriminatorValue)

	err = pageIterator.Iterate(func(pageItem interface{}) bool {
		signIn := pageItem.(models.SignInable)

		d.StreamListItem(ctx, &ADSignInReportInfo{signIn})

		// Context can be cancelled due to manual cancellation or the limit has been hit
		if d.QueryStatus.RowsRemaining(ctx) == 0 {
			return false
		}

		return true
	})

	return nil, nil
}

//// Hydrate Functions

func getAdSignInReportTest(ctx context.Context, d *plugin.QueryData, h *plugin.HydrateData) (interface{}, error) {
	signInID := d.KeyColumnQuals["id"].GetStringValue()
	if signInID == "" {
		return nil, nil
	}

	// Create client
	client, _, err := GetGraphClient(ctx, d)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("error creating client: %v", err))
	}

	signIn, err := client.AuditLogs().SignInsById(signInID).Get()
	if err != nil {
		errObj := getErrorObject(err)
		if isResourceNotFound(errObj) {
			return nil, nil
		}
		return nil, errors.New(fmt.Sprintf("failed to get signIn log. Code: %s Message: %s", errObj.Code, errObj.Message))
	}

	return &ADSignInReportInfo{signIn}, nil
}
