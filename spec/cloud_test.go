package spec

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/mackerelio/mackerel-client-go"

	"github.com/mackerelio/mackerel-agent/config"
)

type mockCloudMetaGenerator struct {
	metadata         *mackerel.Cloud
	customIdentifier string
}

func (g *mockCloudMetaGenerator) Generate() (*mackerel.Cloud, error) {
	return g.metadata, nil
}

func (g *mockCloudMetaGenerator) SuggestCustomIdentifier() (string, error) {
	return g.customIdentifier, nil
}

type mockAzureCloudMetaGenerator struct {
	mockCloudMetaGenerator
	isAzureVM bool
}

func (g *mockAzureCloudMetaGenerator) IsAzureVM(ctx context.Context) bool {
	return g.isAzureVM
}

type mockEC2CloudMetaGenerator struct {
	mockCloudMetaGenerator
	isEC2 bool
}

func (g *mockEC2CloudMetaGenerator) IsEC2(ctx context.Context) bool {
	return g.isEC2
}

type mockGCECloudMetaGenerator struct {
	mockCloudMetaGenerator
	isGCE bool
}

func (g *mockGCECloudMetaGenerator) IsGCE(ctx context.Context) bool {
	return g.isGCE
}

func TestCloudGenerate(t *testing.T) {
	generator := &mockCloudMetaGenerator{
		metadata: &mackerel.Cloud{
			Provider: "mock",
			MetaData: map[string]string{
				"mockKey": "mockValue",
			},
		},
		customIdentifier: "mock-generated-identifier.example.com",
	}
	g := &CloudGenerator{generator}

	value, err := g.Generate()
	if err != nil {
		t.Errorf("should not raise error: %s", err)
	}

	cloud, typeOk := value.(*mackerel.Cloud)
	if !typeOk {
		t.Errorf("value should be *mackerel.Cloud. %+v", value)
	}

	if cloud.Provider != "mock" {
		t.Errorf("Unexpected Provider: %s", cloud.Provider)
	}

	metadata, typeOk := cloud.MetaData.(map[string]string)
	if !typeOk {
		t.Errorf("MetaData should be map. %+v", cloud.MetaData)
	}

	if metadata["mockKey"] != "mockValue" {
		t.Errorf("Unexpected metadata: %s", metadata["mockKey"])
	}

	customIdentifier, err := g.SuggestCustomIdentifier()
	if err != nil {
		t.Errorf("should not raise error: %s", err)
	}

	if customIdentifier != "mock-generated-identifier.example.com" {
		t.Errorf("Unexpected customIdentifier: %s", customIdentifier)
	}
}

func TestEC2Generator(t *testing.T) {
	handler := func(res http.ResponseWriter, req *http.Request) {
		// XXX: should be refined by removing path from ec2BaseURL
		if req.URL.Path == "/instance-id" {
			fmt.Fprint(res, "i-4f90d537")
		} else {
			http.Error(res, "not found", 404)
		}
	}
	ts := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		handler(res, req)
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Errorf("should not raise error: %s", err)
	}
	g := &EC2Generator{u}

	cloud, err := g.Generate()
	if err != nil {
		t.Errorf("should not raise error: %s", err)
	}

	metadata, typeOk := cloud.MetaData.(map[string]string)
	if !typeOk {
		t.Errorf("MetaData should be map. %+v", cloud.MetaData)
	}

	if metadata["instance-id"] != "i-4f90d537" {
		t.Errorf("Unexpected metadata: %s", metadata["instance-id"])
	}

	customIdentifier, err := g.SuggestCustomIdentifier()
	if err != nil {
		t.Errorf("should not raise error: %s", err)
	}

	if customIdentifier != "i-4f90d537.ec2.amazonaws.com" {
		t.Errorf("Unexpected customIdentifier: %s", customIdentifier)
	}
}

func TestEC2SuggestCustomIdentifier_ChangingHttpStatus(t *testing.T) {
	i := 0
	threshold := 100
	handler := func(res http.ResponseWriter, req *http.Request) {
		if i < threshold {
			http.Error(res, "not found", 404)
		} else {
			fmt.Fprint(res, "i-4f90d537")
		}
		i++
	}
	ts := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		handler(res, req)
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Errorf("should not raise error: %s", err)
	}
	g := &EC2Generator{u}

	// 404, 404, 404 => give up
	{
		_, err := g.SuggestCustomIdentifier()
		if err == nil {
			t.Errorf("should raise error: %s", err)
		}
	}
	i = 0
	threshold = 0
	// 200 => ok
	{
		customIdentifier, err := g.SuggestCustomIdentifier()
		if err != nil {
			t.Errorf("should not raise error: %s", err)
		}
		if customIdentifier != "i-4f90d537.ec2.amazonaws.com" {
			t.Error("customIdentifier mismatch")
		}
	}
	i = 0
	threshold = 1
	// 404, 200 => ok
	{
		customIdentifier, err := g.SuggestCustomIdentifier()
		if err != nil {
			t.Errorf("should not raise error: %s", err)
		}
		if customIdentifier != "i-4f90d537.ec2.amazonaws.com" {
			t.Error("customIdentifier mismatch")
		}
	}
	i = 0
	threshold = 3
	// 404, 404, 404(give up), 200, ...
	{
		_, err := g.SuggestCustomIdentifier()
		if err == nil {
			t.Errorf("should raise error: %s", err)
		}
	}
}

func TestGCEGenerate(t *testing.T) {
	// curl "http://metadata.google.internal./computeMetadata/v1/?recursive=true" -H "Metadata-Flavor: Google"
	sampleJSON := []byte(`{
	  "instance": {
		"attributes": {},
		"cpuPlatform": "Intel Ivy Bridge",
		"description": "",
		"disks": [
		  {
			"deviceName": "gce-1",
			"index": 0,
			"mode": "READ_WRITE",
			"type": "PERSISTENT"
		  }
		],
		"hostname": "gce-1.c.dummyproj-987.internal",
		"id": 4567890123456789123,
		"image": "",
		"machineType": "projects/1234567890123/machineTypes/g1-small",
		"maintenanceEvent": "NONE",
		"networkInterfaces": [
		  {
			"accessConfigs": [
			  {
				"externalIp": "203.0.113.1",
				"type": "ONE_TO_ONE_NAT"
			  }
			],
			"forwardedIps": [],
			"ip": "192.0.2.1",
			"network": "projects/1234567890123/networks/default"
		  }
		],
		"scheduling": {
		  "automaticRestart": "TRUE",
		  "onHostMaintenance": "MIGRATE"
		},
		"serviceAccounts": {
		  "1234567890123-compute@developer.gserviceaccount.com": {
			"aliases": [
			  "default"
			],
			"email": "1234567890123-compute@developer.gserviceaccount.com",
			"scopes": [
			  "https://www.googleapis.com/auth/devstorage.read_only",
			  "https://www.googleapis.com/auth/logging.write"
			]
		  },
		  "default": {
			"aliases": [
			  "default"
			],
			"email": "1234567890123-compute@developer.gserviceaccount.com",
			"scopes": [
			  "https://www.googleapis.com/auth/devstorage.read_only",
			  "https://www.googleapis.com/auth/logging.write"
			]
		  }
		},
		"tags": [],
		"virtualClock": {
		  "driftToken": "12345678901234567890"
		},
		"zone": "projects/1234567890123/zones/asia-east1-a"
	  },
	  "project": {
		"attributes": {
		  "google-compute-default-region": "us-central1",
		  "google-compute-default-zone": "us-central1-f",
		  "sshKeys": "dummy_user:ssh-rsa AAAhogehoge google-ssh {\"userName\":\"dummy_user@example.com\",\"expireOn\":\"2015-07-12T11:11:43+0000\"}\ndummy_user:ecdsa-sha2-nistp256 AAAhogefuga google-ssh {\"userName\":\"dummy_user@example.com\",\"expireOn\":\"2015-07-12T11:11:39+0000\"}\n"
		},
		"numericProjectId": 1234567890123,
		"projectId": "dummyprof-987"
	  }
	}`)

	var data gceMeta
	json.Unmarshal(sampleJSON, &data)

	if !reflect.DeepEqual(data.Instance, &gceInstance{
		Zone:         "projects/1234567890123/zones/asia-east1-a",
		InstanceType: "projects/1234567890123/machineTypes/g1-small",
		Hostname:     "gce-1.c.dummyproj-987.internal",
		InstanceID:   4567890123456789123,
	}) {
		t.Errorf("data.Instance should be assigned")
	}

	if !reflect.DeepEqual(data.Project, &gceProject{
		ProjectID:        "dummyprof-987",
		NumericProjectID: 1234567890123,
	}) {
		t.Errorf("data.Project should be assigned")
	}

	if d := data.toGeneratorMeta(); !reflect.DeepEqual(d, map[string]string{
		"zone":          "asia-east1-a",
		"instance-type": "g1-small",
		"hostname":      "gce-1.c.dummyproj-987.internal",
		"instance-id":   "4567890123456789123",
		"projectId":     "dummyprof-987",
	}) {
		t.Errorf("data.Project should be assigned")
	}

}

type slowCloudMetaGenerator struct {
	mockCloudMetaGenerator
}

func (g *slowCloudMetaGenerator) IsEC2(ctx context.Context) bool {
	time.Sleep(2 * time.Second)
	return true
}

func (g *slowCloudMetaGenerator) IsGCE(ctx context.Context) bool {
	time.Sleep(2 * time.Second)
	return true
}

func TestCloudGeneratorSuggester(t *testing.T) {
	conf := config.Config{}
	// none
	{
		suggester := &cloudGeneratorSuggester{
			ec2Generator:     &mockEC2CloudMetaGenerator{isEC2: false},
			gceGenerator:     &mockGCECloudMetaGenerator{isGCE: false},
			azureVMGenerator: &mockAzureCloudMetaGenerator{isAzureVM: false},
		}
		cGen := suggester.Suggest(&conf)
		if cGen != nil {
			t.Errorf("cGen should be nil but, %s", cGen)
		}
	}

	// EC2
	{
		suggester := &cloudGeneratorSuggester{
			ec2Generator:     &mockEC2CloudMetaGenerator{isEC2: true},
			gceGenerator:     &mockGCECloudMetaGenerator{isGCE: false},
			azureVMGenerator: &mockAzureCloudMetaGenerator{isAzureVM: false},
		}
		cGen := suggester.Suggest(&conf)
		if cGen == nil {
			t.Errorf("cGen should not be nil.")
		}

		_, ok := cGen.CloudMetaGenerator.(ec2Generator)
		if !ok {
			t.Errorf("cGen should be ec2Generator")
		}
	}

	// GCE
	{
		suggester := &cloudGeneratorSuggester{
			ec2Generator:     &mockEC2CloudMetaGenerator{isEC2: false},
			gceGenerator:     &mockGCECloudMetaGenerator{isGCE: true},
			azureVMGenerator: &mockAzureCloudMetaGenerator{isAzureVM: false},
		}
		cGen := suggester.Suggest(&conf)
		if cGen == nil {
			t.Errorf("cGen should not be nil.")
		}

		_, ok := cGen.CloudMetaGenerator.(gceGenerator)
		if !ok {
			t.Errorf("cGen should be gceGenerator")
		}
	}

	// AzureVM
	{
		suggester := &cloudGeneratorSuggester{
			ec2Generator:     &mockEC2CloudMetaGenerator{isEC2: false},
			gceGenerator:     &mockGCECloudMetaGenerator{isGCE: false},
			azureVMGenerator: &mockAzureCloudMetaGenerator{isAzureVM: true},
		}
		cGen := suggester.Suggest(&conf)
		if cGen == nil {
			t.Errorf("cGen should not be nil.")
		}

		_, ok := cGen.CloudMetaGenerator.(azureVMGenerator)
		if !ok {
			t.Errorf("cGen should be azureVMGenerator")
		}
	}

	// multiple generators are available, but suggest the first responded one (in this case Azure)
	{
		suggester := &cloudGeneratorSuggester{
			ec2Generator:     &slowCloudMetaGenerator{},
			gceGenerator:     &slowCloudMetaGenerator{},
			azureVMGenerator: &mockAzureCloudMetaGenerator{isAzureVM: true},
		}
		cGen := suggester.Suggest(&conf)
		if cGen == nil {
			t.Errorf("cGen should not be nil.")
		}

		_, ok := cGen.CloudMetaGenerator.(azureVMGenerator)
		if !ok {
			t.Errorf("cGen should be azureVMGenerator")
		}
	}
}

func TestCloudGeneratorSuggester_CloudPlatformSpecified(t *testing.T) {
	suggester := &cloudGeneratorSuggester{
		ec2Generator:     &mockEC2CloudMetaGenerator{isEC2: false},
		gceGenerator:     &mockGCECloudMetaGenerator{isGCE: false},
		azureVMGenerator: &mockAzureCloudMetaGenerator{isAzureVM: false},
	}
	{
		conf := config.Config{
			CloudPlatform: config.CloudPlatformNone,
		}

		cGen := suggester.Suggest(&conf)
		if cGen != nil {
			t.Errorf("cGen should be nil.")
		}
	}

	{
		conf := config.Config{
			CloudPlatform: config.CloudPlatformEC2,
		}

		cGen := suggester.Suggest(&conf)
		if cGen == nil {
			t.Errorf("cGen should not be nil.")
		}

		_, ok := cGen.CloudMetaGenerator.(ec2Generator)
		if !ok {
			t.Errorf("cGen should be ec2Generator")
		}
	}

	{
		conf := config.Config{
			CloudPlatform: config.CloudPlatformGCE,
		}

		cGen := suggester.Suggest(&conf)
		if cGen == nil {
			t.Errorf("cGen should not be nil.")
		}

		_, ok := cGen.CloudMetaGenerator.(gceGenerator)
		if !ok {
			t.Errorf("cGen should be gceGenerator")
		}
	}

	{
		conf := config.Config{
			CloudPlatform: config.CloudPlatformAzureVM,
		}

		cGen := suggester.Suggest(&conf)
		if cGen == nil {
			t.Errorf("cGen should not be nil.")
		}

		_, ok := cGen.CloudMetaGenerator.(azureVMGenerator)
		if !ok {
			t.Errorf("cGen should be azureVMGenerator")
		}
	}
}

func TestCloudGeneratorSuggester_Public(t *testing.T) {
	{
		gen, ok := CloudGeneratorSuggester.ec2Generator.(*EC2Generator)
		if !ok {
			t.Error("EC2Generator should be injected as ec2Generator")
		}
		if gen.baseURL.String() != ec2BaseURL.String() {
			t.Error("real baseURL should be embedded to ec2Generator")
		}
	}
	{
		gen, ok := CloudGeneratorSuggester.gceGenerator.(*GCEGenerator)
		if !ok {
			t.Error("GCEGenerator should be injected as gceGenerator")
		}
		if gen.metaURL.String() != gceMetaURL.String() {
			t.Error("real metaURL should be embedded to gceGenerator")
		}
	}
	{
		gen, ok := CloudGeneratorSuggester.azureVMGenerator.(*AzureVMGenerator)
		if !ok {
			t.Error("AzureVMGenerator should be injected as azureVMGenerator")
		}
		if gen.baseURL.String() != azureVMBaseURL.String() {
			t.Error("real baseURL should be embedded to azureVMGenerator ")
		}
	}
}
