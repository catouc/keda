/*
Copyright 2022 The KEDA Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resolver

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault"
	az "github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/kedacore/keda/v2/pkg/scalers/azure"
)

type AzureKeyVaultHandler struct {
	vault          *kedav1alpha1.AzureKeyVault
	keyvaultClient *keyvault.BaseClient
}

func NewAzureKeyVaultHandler(v *kedav1alpha1.AzureKeyVault) *AzureKeyVaultHandler {
	return &AzureKeyVaultHandler{
		vault: v,
	}
}

func (vh *AzureKeyVaultHandler) Initialize(ctx context.Context, client client.Client, logger logr.Logger, triggerNamespace string) error {
	clientID := vh.vault.Credentials.ClientID
	tenantID := vh.vault.Credentials.TenantID

	clientSecretName := vh.vault.Credentials.ClientSecret.ValueFrom.SecretKeyRef.Name
	clientSecretKey := vh.vault.Credentials.ClientSecret.ValueFrom.SecretKeyRef.Key
	clientSecret := resolveAuthSecret(ctx, client, logger, clientSecretName, triggerNamespace, clientSecretKey)

	clientCredentialsConfig := auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID)

	keyvaultResourceURL, activeDirectoryEndpoint, err := vh.getPropertiesForCloud()
	if err != nil {
		return err
	}
	clientCredentialsConfig.Resource = keyvaultResourceURL
	clientCredentialsConfig.AADEndpoint = activeDirectoryEndpoint

	authorizer, err := clientCredentialsConfig.Authorizer()
	if err != nil {
		return err
	}

	keyvaultClient := keyvault.New()
	keyvaultClient.Authorizer = authorizer

	vh.keyvaultClient = &keyvaultClient

	return nil
}

func (vh *AzureKeyVaultHandler) Read(ctx context.Context, secretName string, version string) (string, error) {
	result, err := vh.keyvaultClient.GetSecret(ctx, vh.vault.VaultURI, secretName, version)
	if err != nil {
		return "", err
	}

	return *result.Value, nil
}

func (vh *AzureKeyVaultHandler) getPropertiesForCloud() (string, string, error) {
	cloud := vh.vault.Cloud

	if cloud == nil {
		return az.PublicCloud.ResourceIdentifiers.KeyVault, az.PublicCloud.ActiveDirectoryEndpoint, nil
	}

	if strings.EqualFold(cloud.Type, azure.PrivateCloud) {
		if cloud.KeyVaultResourceURL == "" || cloud.ActiveDirectoryEndpoint == "" {
			err := fmt.Errorf("properties keyVaultResourceURL and activeDirectoryEndpoint must be provided for cloud %s",
				azure.PrivateCloud)
			return "", "", err
		}

		return cloud.KeyVaultResourceURL, cloud.ActiveDirectoryEndpoint, nil
	}

	env, err := az.EnvironmentFromName(cloud.Type)
	if err != nil {
		return "", "", err
	}

	return env.ResourceIdentifiers.KeyVault, env.ActiveDirectoryEndpoint, nil
}
