package azuread

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iancoleman/strcase"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/auditlogs/directoryaudits"
	"github.com/microsoftgraph/msgraph-sdk-go/models"

	"github.com/turbot/steampipe-plugin-sdk/v5/grpc/proto"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin/transform"
)

//// TABLE DEFINITION

func tableAzureAdDirectoryAuditReport(_ context.Context) *plugin.Table {
	return &plugin.Table{
		Name:        "azuread_directory_audit_report",
		Description: "Represents the list of audit logs generated by Azure Active Directory.",
		Get: &plugin.GetConfig{
			Hydrate: getAdDirectoryAuditReport,
			IgnoreConfig: &plugin.IgnoreConfig{
				ShouldIgnoreErrorFunc: isIgnorableErrorPredicate([]string{"Request_ResourceNotFound", "Invalid object identifier"}),
			},
			KeyColumns: plugin.SingleColumn("id"),
		},
		List: &plugin.ListConfig{
			Hydrate: listAdDirectoryAuditReports,
			KeyColumns: plugin.KeyColumnSlice{
				// Key fields
				{Name: "activity_date_time", Require: plugin.Optional, Operators: []string{">", ">=", "=", "<", "<="}},
				{Name: "activity_display_name", Require: plugin.Optional},
				{Name: "category", Require: plugin.Optional},
				{Name: "correlation_id", Require: plugin.Optional},
				{Name: "filter", Require: plugin.Optional},
				{Name: "result", Require: plugin.Optional},
			},
		},

		Columns: []*plugin.Column{
			{Name: "id", Type: proto.ColumnType_STRING, Description: "Indicates the unique ID for the activity.", Transform: transform.FromMethod("GetId")},
			{Name: "activity_date_time", Type: proto.ColumnType_TIMESTAMP, Description: "Indicates the date and time the activity was performed.", Transform: transform.FromMethod("GetActivityDateTime")},
			{Name: "activity_display_name", Type: proto.ColumnType_STRING, Description: "Indicates the activity name or the operation name.", Transform: transform.FromMethod("GetActivityDisplayName")},
			{Name: "category", Type: proto.ColumnType_STRING, Description: "Indicates which resource category that's targeted by the activity.", Transform: transform.FromMethod("GetCategory")},
			{Name: "correlation_id", Type: proto.ColumnType_STRING, Description: "Indicates a unique ID that helps correlate activities that span across various services. Can be used to trace logs across services.", Transform: transform.FromMethod("GetCorrelationId")},
			{Name: "logged_by_service", Type: proto.ColumnType_STRING, Description: "Indicates information on which service initiated the activity (For example: Self-service Password Management, Core Directory, B2C, Invited Users, Microsoft Identity Manager, Privileged Identity Management.", Transform: transform.FromMethod("GetLoggedByService")},
			{Name: "operation_type", Type: proto.ColumnType_STRING, Description: "Indicates the type of operation that was performed. The possible values include but are not limited to the following: Add, Assign, Update, Unassign, and Delete.", Transform: transform.FromMethod("GetOperationType")},
			{Name: "result", Type: proto.ColumnType_STRING, Description: "Indicates the result of the activity. Possible values are: success, failure, timeout, unknownFutureValue.", Transform: transform.FromMethod("DirectoryAuditResult")},
			{Name: "result_reason", Type: proto.ColumnType_STRING, Description: "Indicates the reason for failure if the result is failure or timeout.", Transform: transform.FromMethod("GetResultReason")},

			// JSON fields
			{Name: "additional_details", Type: proto.ColumnType_JSON, Description: "Indicates additional details on the activity.", Transform: transform.FromMethod("DirectoryAuditAdditionalDetails")},
			{Name: "initiated_by", Type: proto.ColumnType_JSON, Description: "Indicates information about the user or app initiated the activity.", Transform: transform.FromMethod("DirectoryAuditInitiatedBy")},
			{Name: "target_resources", Type: proto.ColumnType_JSON, Description: "Indicates information on which resource was changed due to the activity. Target Resource Type can be User, Device, Directory, App, Role, Group, Policy or Other.", Transform: transform.FromMethod("DirectoryAuditTargetResources")},

			// Standard columns
			{Name: "title", Type: proto.ColumnType_STRING, Description: ColumnDescriptionTitle, Transform: transform.FromMethod("GetId")},
			{Name: "tenant_id", Type: proto.ColumnType_STRING, Description: ColumnDescriptionTenant, Hydrate: plugin.HydrateFunc(getTenant).WithCache(), Transform: transform.FromValue()},
			{Name: "filter", Type: proto.ColumnType_STRING, Transform: transform.FromQual("filter"), Description: "Odata query to search for directory audit reports."},
		},
	}
}

//// LIST FUNCTION

func listAdDirectoryAuditReports(ctx context.Context, d *plugin.QueryData, _ *plugin.HydrateData) (interface{}, error) {
	// Create client
	client, adapter, err := GetGraphClient(ctx, d)
	if err != nil {
		plugin.Logger(ctx).Error("azuread_directory_audit_report.listAdDirectoryAuditReports", "connection_error", err)
		return nil, err
	}

	// List operations
	input := &directoryaudits.DirectoryAuditsRequestBuilderGetQueryParameters{
		Top: Int32(1000),
	}

	// Restrict the limit value to be passed in the query parameter which is not between 1 and 1000, otherwise API will throw an error as follow
	// The limit of '1000' for Top query has been exceeded.
	limit := d.QueryContext.Limit
	if limit != nil {
		if *limit > 0 && *limit < 1000 {
			l := int32(*limit)
			input.Top = Int32(l)
		}
	}

	equalQuals := d.EqualsQuals

	var queryFilter string
	filter := buildDirectoryAuditQueryFilter(equalQuals)

	// Filter by activityDateTime
	if d.Quals["activity_date_time"] != nil {
		for _, q := range d.Quals["activity_date_time"].Quals {
			givenTime := q.Value.GetTimestampValue().AsTime()
			//startTime = q.Value.GetTimestampValue().AsTime().Format(time.RFC3339)

			switch q.Operator {
			case ">":
				startTime := givenTime.Add(time.Second * 1).Format(time.RFC3339)
				filter = append(filter, fmt.Sprintf("activityDateTime ge %s", startTime))
			case ">=":
				filter = append(filter, fmt.Sprintf("activityDateTime ge %s", givenTime.Format(time.RFC3339)))
			case "=":
				filter = append(filter, fmt.Sprintf("activityDateTime eq %s", givenTime.Format(time.RFC3339)))
			case "<=":
				filter = append(filter, fmt.Sprintf("activityDateTime le %s", givenTime.Format(time.RFC3339)))
			case "<":
				startTime := givenTime.Add(time.Duration(-1) * time.Second).Format(time.RFC3339)
				filter = append(filter, fmt.Sprintf("activityDateTime le %s", startTime))
			}
		}
	}

	if equalQuals["filter"] != nil {
		queryFilter = equalQuals["filter"].GetStringValue()
	}

	if queryFilter != "" {
		input.Filter = &queryFilter
	} else if len(filter) > 0 {
		joinStr := strings.Join(filter, " and ")
		input.Filter = &joinStr
	}

	options := &directoryaudits.DirectoryAuditsRequestBuilderGetRequestConfiguration{
		QueryParameters: input,
	}

	result, err := client.AuditLogs().DirectoryAudits().Get(ctx, options)
	if err != nil {
		errObj := getErrorObject(err)
		plugin.Logger(ctx).Error("listAdDirectoryAuditReports", "list_directory_audit_report_error", errObj)
		return nil, errObj
	}

	pageIterator, err := msgraphcore.NewPageIterator(result, adapter, models.CreateSignInCollectionResponseFromDiscriminatorValue)
	if err != nil {
		plugin.Logger(ctx).Error("listAdDirectoryAuditReports", "create_iterator_instance_error", err)
		return nil, err
	}

	err = pageIterator.Iterate(ctx, func(pageItem interface{}) bool {

		if directoryAudit, ok := pageItem.(models.DirectoryAuditable); ok {
			d.StreamListItem(ctx, &ADDirectoryAuditReportInfo{directoryAudit})

		}
			// Context can be cancelled due to manual cancellation or the limit has been hit
			return d.RowsRemaining(ctx) != 0
	})
	if err != nil {
		plugin.Logger(ctx).Error("listAdDirectoryAuditReports", "paging_error", err)
		return nil, err
	}

	return nil, nil
}

//// HYDRATE FUNCTIONS

func getAdDirectoryAuditReport(ctx context.Context, d *plugin.QueryData, h *plugin.HydrateData) (interface{}, error) {
	directoryAuditID := d.EqualsQuals["id"].GetStringValue()
	if directoryAuditID == "" {
		return nil, nil
	}

	// Create client
	client, _, err := GetGraphClient(ctx, d)
	if err != nil {
		plugin.Logger(ctx).Error("azuread_directory_audit_report.getAdDirectoryAuditReport", "connection_error", err)
		return nil, err
	}

	directoryAudit, err := client.AuditLogs().DirectoryAuditsById(directoryAuditID).Get(ctx, nil)
	if err != nil {
		errObj := getErrorObject(err)
		plugin.Logger(ctx).Error("getAdDirectoryAuditReport", "get_directory_audit_report_error", errObj)
		return nil, errObj
	}

	return &ADDirectoryAuditReportInfo{directoryAudit}, nil
}

func buildDirectoryAuditQueryFilter(equalQuals plugin.KeyColumnEqualsQualMap) []string {
	filters := []string{}

	filterQuals := map[string]string{
		"activity_display_name": "string",
		"category":              "string",
		"correlation_id":        "string",
		"result":                "string",
	}

	for qual := range filterQuals {
		if equalQuals[qual] != nil {
			filters = append(filters, fmt.Sprintf("%s eq '%s'", strcase.ToCamel(qual), equalQuals[qual].GetStringValue()))
		}
	}

	return filters
}
