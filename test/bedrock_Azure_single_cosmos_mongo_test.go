package test

import (
	"fmt"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/otiai10/copy"

	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestIT_Bedrock_Azure_Single_KV_Cosmos_Mongo_DB_Test(t *testing.T) {
	t.Parallel()

	//Generate common-infra resources for integration use
	uniqueID := strings.ToLower(random.UniqueId())
	k8sName := fmt.Sprintf("gtestk8s-%s", uniqueID)
	addressSpace := "10.39.0.0/16"
	kvName := k8sName + "-kv"
	kvRG := kvName + "-rg"
	location := os.Getenv("DATACENTER_LOCATION")
	clientid := os.Getenv("ARM_CLIENT_ID")
	subnetName := k8sName + "-subnet"
	vnetName := k8sName + "-vnet"

	//Generate common-infra backend for tf.state files to be persisted in azure storage account
	backendName := os.Getenv("ARM_BACKEND_STORAGE_NAME")
	backendKey := os.Getenv("ARM_BACKEND_STORAGE_KEY")
	backendContainer := os.Getenv("ARM_BACKEND_STORAGE_CONTAINER")
	backendTfstatekey := k8sName + "-tfstatekey"

	//Copy env directories as needed to avoid conflicting with other running tests
	azureCommonInfraFolder := "../cluster/test-temp-envs/azure-common-infra-" + k8sName
	copy.Copy("../cluster/environments/azure-common-infra", azureCommonInfraFolder)

	//Create the common resource group
	cmd := exec.Command("az", "group", "create", "-n", kvRG, "-l", location)
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
		os.Exit(-1)
	}

	//Specify the test case folder and "-var" option mapping for the backend
	common_backend_tfOptions := &terraform.Options{
		TerraformDir: azureCommonInfraFolder,
		BackendConfig: map[string]interface{}{
			"storage_account_name": backendName,
			"access_key":           backendKey,
			"container_name":       backendContainer,
			"key":                  "common_" + backendTfstatekey,
		},
	}

	//Specify the test case folder and "-var" option mapping
	common_tfOptions := &terraform.Options{
		TerraformDir: azureCommonInfraFolder,
		Upgrade:      true,
		Vars: map[string]interface{}{
			"address_space":                  addressSpace,
			"keyvault_name":                  kvName,
			"global_resource_group_name":     kvRG,
			"service_principal_id":           clientid,
			"subnet_name":                    subnetName,
			"subnet_prefix":                  addressSpace,
			"vnet_name":                      vnetName,
		},
	}

	//Terraform init, apply, output, and defer destroy for common-infra bedrock environment
	defer terraform.Destroy(t, common_tfOptions)
	terraform.Init(t, common_backend_tfOptions)
	terraform.Apply(t, common_tfOptions)

	// Generate azure single environment using resources generated from common-infra
        dnsprefix := k8sName + "-dns"
        clientsecret := os.Getenv("ARM_CLIENT_SECRET")
	k8sRG := k8sName + "-rg"
	publickey := os.Getenv("public_key")
	sshkey := os.Getenv("ssh_key")
	cosmos_db_name := k8sName + "-cosmosdb"
	mongo_db_name := k8sName + "-mongodb"

	//Copy env directories as needed to avoid conflicting with other running tests
	azureSingleKeyvaultFolder := "../cluster/test-temp-envs/azure-single-keyvault-cosmos-mongo-db-simple-" + k8sName
	copy.Copy("../cluster/environments/azure-single-keyvault-cosmos-mongo-db-simple", azureSingleKeyvaultFolder)

	//Create the cluster resource group
	cmd2 := exec.Command("az", "group", "create", "-n", k8sRG, "-l", location)
	err2 := cmd2.Run()
	if err != nil {
		log.Fatal(err2)
		os.Exit(-1)
	}

	//Specify the test case folder and "-var" option mapping for the environment backend
	k8s_backend_tfOptions := &terraform.Options{
		TerraformDir: azureSingleKeyvaultFolder,
		BackendConfig: map[string]interface{}{
			"storage_account_name": backendName,
			"access_key":           backendKey,
			"container_name":       backendContainer,
			"key":                  backendTfstatekey,
		},
	}

	// Specify the test case folder and "-var" options
	k8s_tfOptions := &terraform.Options{
		TerraformDir: azureSingleKeyvaultFolder,
		Upgrade:      true,
		Vars: map[string]interface{}{
			"address_space":            addressSpace,
			"agent_vm_count":           "3",
			"agent_vm_size":            "Standard_D2s_v3",
			"cluster_name":             k8sName,
			"dns_prefix":               dnsprefix,
			"gitops_ssh_url":           "git@github.com:timfpark/fabrikate-cloud-native-manifests.git",
			"gitops_ssh_key":           sshkey,
			"keyvault_name":            kvName,
			"keyvault_resource_group":  kvRG,
			"resource_group_name":      k8sRG,
			"resource_group_location":  location,
			"ssh_public_key":           publickey,
			"service_principal_id":     clientid,
			"service_principal_secret": clientsecret,
			"subnet_prefixes":          "10.39.0.0/16",
			"subnet_name":              subnetName,
			"vnet_name":                vnetName,
			"cosmos_db_name":           cosmos_db_name,
			"mongo_db_name":            mongo_db_name,
		},
	}

	//Terraform init, apply, output, and defer destroy on azure-single-keyvault-cosmos-mongo-db-simple bedrock environment
	defer terraform.Destroy(t, k8s_tfOptions)
	terraform.Init(t, k8s_backend_tfOptions)
	terraform.Apply(t, k8s_tfOptions)

	//Obtain Kube_config file from module output
	os.Setenv("KUBECONFIG", azureSingleKeyvaultFolder+"/output/bedrock_kube_config")
	kubeConfig := os.Getenv("KUBECONFIG")
	options := k8s.NewKubectlOptions("", kubeConfig)

	//Test Case 1: Verify Flux namespace
	fmt.Println("Test case 1: Verifying flux namespace")
	_flux, fluxErr := k8s.RunKubectlAndGetOutputE(t, options, "get", "po", "--namespace=flux")
	if fluxErr != nil || !strings.Contains(_flux, "flux") {
		t.Fatal(fluxErr)
	} else {
		fmt.Println("Flux verification complete")
	}

	//Test Case 2: Verify keyvault namespace flex
	fmt.Println("Test case 2: Verifying flexvolume and kv namespace")
	_flex, flexErr := k8s.RunKubectlAndGetOutputE(t, options, "get", "po", "--namespace=kv")
	if flexErr != nil || !strings.Contains(_flex, "keyvault-flexvolume") {
		t.Fatal(flexErr)
	} else {
		fmt.Println("Flexvolume verification complete")
	}

	//Test Case 3: Verify Cosmos/MongoDB
	fmt.Println("Test case 3: Verifying Cosmos/MongoDB deployment")
	cosmos_db_key := terraform.Output(t, k8s_tfOptions, "azure_cosmos_db_primary_master_key")
	cmd3 := exec.Command("az", "cosmosdb", "database", "exists", "--name", cosmos_db_name, "--key", cosmos_db_key, "--db-name", mongo_db_name)
	out, cosmosMongoErr := cmd3.CombinedOutput()
	if cosmosMongoErr != nil {
		t.Fatal(cosmosMongoErr)
	} else if !strings.Contains(string(out), "true") {
		t.Fatal(cosmosMongoErr)
	} else {
		fmt.Println("CosmosDB with MongoDB verification complete.")
	}
}
