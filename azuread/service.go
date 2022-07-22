package azuread

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/manicminer/hamilton/auth"
	"github.com/manicminer/hamilton/environments"
	a "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdkgo "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/turbot/steampipe-plugin-sdk/v3/plugin"
)

// Session info
type Session struct {
	TenantID   string
	Authorizer auth.Authorizer
}

/* GetNewSession creates an session configured from (~/.steampipe/config, environment variables and CLI) in the order:
1. Client secret
2. Client certificate
3. Username and password
4. MSI
5. CLI
*/
func GetNewSession(ctx context.Context, d *plugin.QueryData) (sess *Session, err error) {
	logger := plugin.Logger(ctx)

	// Have we already created and cached the session?
	// Hamilton SDK already acquires a new token when expired, so don't handle here again
	sessionCacheKey := "GetNewSession"
	if cachedData, ok := d.ConnectionManager.Cache.Get(sessionCacheKey); ok {
		return cachedData.(*Session), nil
	}

	azureADConfig := GetConfig(d.Connection)
	var tenantID string
	authMethod, authConfig, err := getApplicableAuthorizationDetails(ctx, azureADConfig)
	if err != nil {
		logger.Debug("GetNewSession__", "getApplicableAuthorizationDetails error", err)
		return nil, err
	}

	if authConfig.TenantID != "" {
		tenantID = authConfig.TenantID
	}

	authorizer, err := authConfig.NewAuthorizer(ctx, auth.MsGraph)
	if err != nil {
		log.Fatal(err)
	}

	if authMethod == "CLI" {
		tenantID, err = getTenantFromCLI()
		if err != nil {
			logger.Debug("GetNewSession__", "getTenantFromCLI error", err)
			return nil, err
		}
	}

	sess = &Session{
		Authorizer: authorizer,
		TenantID:   tenantID,
	}

	// Save session into cache
	d.ConnectionManager.Cache.Set(sessionCacheKey, sess)

	return sess, err
}

func getApplicableAuthorizationDetails(ctx context.Context, config azureADConfig) (authMethod string, authConfig auth.Config, err error) {

	var environment, tenantID, clientID, clientSecret, certificatePath, certificatePassword, msiEndpoint string
	var enableMsi bool
	// username, password string
	if config.TenantID != nil {
		tenantID = *config.TenantID
	} else {
		tenantID = os.Getenv("AZURE_TENANT_ID")
	}

	if config.Environment != nil {
		environment = *config.Environment
	} else {
		environment = os.Getenv("AZURE_ENVIRONMENT")
	}

	// Can be "AZURECHINACLOUD", "AZUREGERMANCLOUD", "AZUREPUBLICCLOUD", "AZUREUSGOVERNMENTCLOUD"
	switch environment {
	case "AZURECHINACLOUD":
		authConfig.Environment = environments.China
	case "AZUREUSGOVERNMENTCLOUD":
		authConfig.Environment = environments.USGovernmentL4
	case "AZUREGERMANCLOUD":
		authConfig.Environment = environments.Germany
	default:
		authConfig.Environment = environments.Global
	}

	// 1. Client secret credentials
	if config.ClientID != nil {
		clientID = *config.ClientID
	} else {
		clientID = os.Getenv("AZURE_CLIENT_ID")
	}

	if config.ClientSecret != nil {
		clientSecret = *config.ClientSecret
	} else {
		clientSecret = os.Getenv("AZURE_CLIENT_SECRET")
	}

	// 2. Client certificate credentials
	if config.CertificatePath != nil {
		certificatePath = *config.CertificatePath
	} else {
		certificatePath = os.Getenv("AZURE_CERTIFICATE_PATH")
	}

	if config.CertificatePassword != nil {
		certificatePassword = *config.CertificatePassword
	} else {
		certificatePassword = os.Getenv("AZURE_CERTIFICATE_PASSWORD")
	}

	// TODO
	// 3. Username and password
	// if config.Username != nil {
	// 	username = *config.Username
	// } else {
	// 	username = os.Getenv("AZURE_USERNAME")
	// }

	// if config.Password != nil {
	// 	password = *config.Password
	// } else {
	// 	password = os.Getenv("AZURE_PASSWORD")
	// }

	// 4. MSI credentials
	msiEndpoint = "http://169.254.169.254/metadata/identity/oauth2/token"
	if config.EnableMsi != nil {
		enableMsi = *config.EnableMsi

		if config.MsiEndpoint != nil {
			msiEndpoint = *config.MsiEndpoint
		}
	}

	// 5. Default to CLI credentials
	authMethod = "CLI"

	if tenantID == "" {
		authMethod = "CLI"
		authConfig.EnableAzureCliToken = true
	} else if tenantID != "" && clientID != "" && clientSecret != "" {
		authConfig.TenantID = tenantID
		authConfig.ClientID = clientID
		authConfig.ClientSecret = clientSecret
		authConfig.EnableClientSecretAuth = true
		authMethod = "EnableClientSecretAuth"
	} else if tenantID != "" && clientID != "" && certificatePath != "" && certificatePassword != "" {
		authConfig.TenantID = tenantID
		authConfig.ClientID = clientID
		authConfig.ClientCertPath = certificatePath
		authConfig.ClientCertPassword = certificatePassword
		authConfig.EnableClientCertAuth = true
		authMethod = "EnableClientCertificateAuth"
	} else if enableMsi {
		authConfig.EnableMsiAuth = true
		authConfig.MsiEndpoint = msiEndpoint
		authConfig.TenantID = tenantID
		authConfig.ClientID = clientID
		authMethod = "EnableMsiAuth"
	}
	return
}

// https://github.com/Azure/go-autorest/blob/3fb5326fea196cd5af02cf105ca246a0fba59021/autorest/azure/cli/token.go#L126
// NewAuthorizerFromCLIWithResource creates an Authorizer configured from Azure CLI 2.0 for local development scenarios.
func getTenantFromCLI() (string, error) {
	// This is the path that a developer can set to tell this class what the install path for Azure CLI is.
	const azureCLIPath = "AzureCLIPath"

	// The default install paths are used to find Azure CLI. This is for security, so that any path in the calling program's Path environment is not used to execute Azure CLI.
	azureCLIDefaultPathWindows := fmt.Sprintf("%s\\Microsoft SDKs\\Azure\\CLI2\\wbin; %s\\Microsoft SDKs\\Azure\\CLI2\\wbin", os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"))

	// Default path for non-Windows.
	const azureCLIDefaultPath = "/bin:/sbin:/usr/bin:/usr/local/bin"

	// Execute Azure CLI to get token
	var cliCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cliCmd = exec.Command(fmt.Sprintf("%s\\system32\\cmd.exe", os.Getenv("windir")))
		cliCmd.Env = os.Environ()
		cliCmd.Env = append(cliCmd.Env, fmt.Sprintf("PATH=%s;%s", os.Getenv(azureCLIPath), azureCLIDefaultPathWindows))
		cliCmd.Args = append(cliCmd.Args, "/c", "az")
	} else {
		cliCmd = exec.Command("az")
		cliCmd.Env = os.Environ()
		cliCmd.Env = append(cliCmd.Env, fmt.Sprintf("PATH=%s:%s", os.Getenv(azureCLIPath), azureCLIDefaultPath))
	}
	cliCmd.Args = append(cliCmd.Args, "account", "get-access-token", "--resource-type=ms-graph", "-o", "json")

	var stderr bytes.Buffer
	cliCmd.Stderr = &stderr

	output, err := cliCmd.Output()
	if err != nil {
		return "", fmt.Errorf("Invoking Azure CLI failed with the following error: %v", err)
	}

	var tokenResponse struct {
		AccessToken string `json:"accessToken"`
		ExpiresOn   string `json:"expiresOn"`
		Tenant      string `json:"tenant"`
		TokenType   string `json:"tokenType"`
	}
	err = json.Unmarshal(output, &tokenResponse)
	if err != nil {
		return "", err
	}

	return tokenResponse.Tenant, nil
}

func GetGraphClient(ctx context.Context, d *plugin.QueryData) (*msgraphsdkgo.GraphServiceClient, *msgraphsdkgo.GraphRequestAdapter, error) {
	logger := plugin.Logger(ctx)

	// Have we already created and cached the session?
	sessionCacheKey := "GetGraphClient"
	if cachedData, ok := d.ConnectionManager.Cache.Get(sessionCacheKey); ok {
		return cachedData.(*msgraphsdkgo.GraphServiceClient), nil, nil
	}

	var tenantID, environment, clientID, clientSecret, certificatePath, certificatePassword string

	azureADConfig := GetConfig(d.Connection)
	if azureADConfig.TenantID != nil {
		tenantID = *azureADConfig.TenantID
	} else {
		tenantID = os.Getenv("AZURE_TENANT_ID")
	}

	if azureADConfig.Environment != nil {
		environment = *azureADConfig.Environment
	} else {
		environment = os.Getenv("AZURE_ENVIRONMENT")
	}

	var enableMsi bool
	if azureADConfig.EnableMsi != nil {
		enableMsi = *azureADConfig.EnableMsi
	}

	// 1. Client secret credentials
	if azureADConfig.ClientID != nil {
		clientID = *azureADConfig.ClientID
	} else {
		clientID = os.Getenv("AZURE_CLIENT_ID")
	}

	if azureADConfig.ClientSecret != nil {
		clientSecret = *azureADConfig.ClientSecret
	} else {
		clientSecret = os.Getenv("AZURE_CLIENT_SECRET")
	}

	// 2. Client certificate credentials
	if azureADConfig.CertificatePath != nil {
		certificatePath = *azureADConfig.CertificatePath
	} else {
		certificatePath = os.Getenv("AZURE_CERTIFICATE_PATH")
	}

	if azureADConfig.CertificatePassword != nil {
		certificatePassword = *azureADConfig.CertificatePassword
	} else {
		certificatePassword = os.Getenv("AZURE_CERTIFICATE_PASSWORD")
	}

	var cloudConfiguration cloud.Configuration
	switch environment {
	case "AZURECHINACLOUD":
		cloudConfiguration = cloud.AzureChina
	case "AZUREUSGOVERNMENTCLOUD":
		cloudConfiguration = cloud.AzureGovernment
	// case "AZUREGERMANCLOUD":
	// 	cloudConfiguration = environments.Germany
	default:
		cloudConfiguration = cloud.AzurePublic
	}

	var cred azcore.TokenCredential
	var err error
	if tenantID == "" { // CLI authentication
		cred, err = azidentity.NewAzureCLICredential(
			&azidentity.AzureCLICredentialOptions{},
		)
		if err != nil {
			logger.Error("GetGraphClient", "credential_error", err)
			return nil, nil, fmt.Errorf("error creating credentials: %w", err)
		}
	} else if tenantID != "" && clientID != "" && clientSecret != "" { // Client secret authentication
		cred, err = azidentity.NewClientSecretCredential(
			tenantID,
			clientID,
			clientSecret,
			&azidentity.ClientSecretCredentialOptions{
				ClientOptions: policy.ClientOptions{
					Cloud: cloudConfiguration,
				},
			},
		)
		if err != nil {
			logger.Error("GetGraphClient", "credential_error", err)
			return nil, nil, fmt.Errorf("error creating credentials: %w", err)
		}
	} else if tenantID != "" && clientID != "" && certificatePath != "" { // Client certificate authentication
		// Load certificate from given path
		loadFile, err := os.ReadFile(certificatePath)
		if err != nil {
			return nil, nil, fmt.Errorf("error reading certificate from %s: %v", certificatePath, err)
		}

		var certs []*x509.Certificate
		var key crypto.PrivateKey
		if certificatePassword == "" {
			certs, key, err = azidentity.ParseCertificates(loadFile, nil)
		} else {
			certs, key, err = azidentity.ParseCertificates(loadFile, []byte(certificatePassword))
		}

		if err != nil {
			return nil, nil, fmt.Errorf("error parsing certificate from %s: %v", certificatePath, err)
		}

		cred, err = azidentity.NewClientCertificateCredential(
			tenantID,
			clientID,
			certs,
			key,
			&azidentity.ClientCertificateCredentialOptions{
				ClientOptions: policy.ClientOptions{
					Cloud: cloudConfiguration,
				},
			},
		)
	} else if enableMsi { // Managed identity authentication
		cred, err = azidentity.NewManagedIdentityCredential(
			&azidentity.ManagedIdentityCredentialOptions{},
		)
	}

	auth, err := a.NewAzureIdentityAuthenticationProvider(cred)
	if err != nil {
		return nil, nil, errors.New(fmt.Sprintf("error creating authentication provider: %v", err))
	}

	adapter, err := msgraphsdkgo.NewGraphRequestAdapter(auth)
	if err != nil {
		return nil, nil, errors.New(fmt.Sprintf("error creating graph adapter: %v", err))
	}
	client := msgraphsdkgo.NewGraphServiceClient(adapter)

	// Save session into cache
	d.ConnectionManager.Cache.Set(sessionCacheKey, client)

	return client, adapter, nil
}
