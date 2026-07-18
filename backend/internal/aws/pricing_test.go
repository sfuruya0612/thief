package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	sptypes "github.com/aws/aws-sdk-go-v2/service/savingsplans/types"
	"github.com/google/go-cmp/cmp"
)

// 実際に AWS Price List API から取得した生 JSON (issue 0045 実装時にライブ検証済み) を
// もとにした、構造を忠実に保ったテスト用ドキュメント。

const ec2PriceDoc = `{
  "product": {"sku": "SKU1", "productFamily": "Compute Instance", "attributes": {
    "instanceType": "m5.large", "operatingSystem": "Linux", "tenancy": "Shared",
    "regionCode": "ap-northeast-1", "usagetype": "APN1-BoxUsage:m5.large",
    "capacitystatus": "Used", "preInstalledSw": "NA"
  }},
  "terms": {
    "OnDemand": {"SKU1.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
      "SKU1.OTC1.RC1": {"rateCode": "SKU1.OTC1.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "0.1240000000"}}
    }}},
    "Reserved": {
      "SKU1.RI1": {"offerTermCode": "RI1",
        "termAttributes": {"LeaseContractLength": "1yr", "OfferingClass": "standard", "PurchaseOption": "No Upfront"},
        "priceDimensions": {"SKU1.RI1.RC1": {"rateCode": "SKU1.RI1.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "0.0780000000"}}}},
      "SKU1.RI2": {"offerTermCode": "RI2",
        "termAttributes": {"LeaseContractLength": "1yr", "OfferingClass": "convertible", "PurchaseOption": "All Upfront"},
        "priceDimensions": {
          "SKU1.RI2.RC1": {"rateCode": "SKU1.RI2.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "0.0000000000"}},
          "SKU1.RI2.RC2": {"rateCode": "SKU1.RI2.RC2", "unit": "Quantity", "pricePerUnit": {"USD": "812"}}
        }}
    }
  }
}`

const rdsPriceDoc = `{
  "product": {"sku": "SKU2", "productFamily": "Database Instance", "attributes": {
    "instanceType": "db.t3.micro", "databaseEngine": "MySQL", "deploymentOption": "Single-AZ",
    "licenseModel": "No license required", "regionCode": "ap-northeast-1", "usagetype": "APN1-InstanceUsage:db.t3.micro"
  }},
  "terms": {
    "OnDemand": {"SKU2.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
      "SKU2.OTC1.RC1": {"rateCode": "SKU2.OTC1.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "0.0260000000"}}
    }}},
    "Reserved": {
      "SKU2.RI1": {"offerTermCode": "RI1",
        "termAttributes": {"LeaseContractLength": "1yr", "OfferingClass": "standard", "PurchaseOption": "No Upfront"},
        "priceDimensions": {"SKU2.RI1.RC1": {"rateCode": "SKU2.RI1.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "0.0202000000"}}}}
    }
  }
}`

const elastiCachePriceDoc = `{
  "product": {"sku": "SKU3", "productFamily": "Cache Instance", "attributes": {
    "instanceType": "cache.t3.micro", "cacheEngine": "Redis",
    "regionCode": "ap-northeast-1", "usagetype": "APN1-NodeUsage:cache.t3.micro"
  }},
  "terms": {
    "OnDemand": {"SKU3.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
      "SKU3.OTC1.RC1": {"rateCode": "SKU3.OTC1.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "0.0260000000"}}
    }}}
  }
}`

const elastiCacheExtendedSupportDoc = `{
  "product": {"sku": "SKU4", "productFamily": "Cache Instance", "attributes": {
    "instanceType": "cache.t3.micro", "cacheEngine": "Redis",
    "regionCode": "ap-northeast-1", "usagetype": "APN1-ExtendedSupportYr3-NodeUsage:cache.t3.micro"
  }},
  "terms": {
    "OnDemand": {"SKU4.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
      "SKU4.OTC1.RC1": {"rateCode": "SKU4.OTC1.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "0.0420000000"}}
    }}}
  }
}`

const ecsFargateVCPUDoc = `{
  "product": {"sku": "SKU5", "productFamily": "Compute", "attributes": {
    "regionCode": "ap-northeast-1", "usagetype": "APN1-Fargate-vCPU-Hours:perCPU", "tenancy": "Shared"
  }},
  "terms": {"OnDemand": {"SKU5.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
    "SKU5.OTC1.RC1": {"rateCode": "SKU5.OTC1.RC1", "unit": "hours", "pricePerUnit": {"USD": "0.0505600000"}}
  }}}}
}`

const ecsFargateGBDoc = `{
  "product": {"sku": "SKU6", "productFamily": "Compute", "attributes": {
    "regionCode": "ap-northeast-1", "usagetype": "APN1-Fargate-GB-Hours", "tenancy": "Shared"
  }},
  "terms": {"OnDemand": {"SKU6.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
    "SKU6.OTC1.RC1": {"rateCode": "SKU6.OTC1.RC1", "unit": "hours", "pricePerUnit": {"USD": "0.0055300000"}}
  }}}}
}`

const ecsFargateEphemeralDoc = `{
  "product": {"sku": "SKU7", "productFamily": "Compute", "attributes": {
    "regionCode": "ap-northeast-1", "usagetype": "APN1-Fargate-EphemeralStorage-GB-Hours"
  }},
  "terms": {"OnDemand": {"SKU7.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
    "SKU7.OTC1.RC1": {"rateCode": "SKU7.OTC1.RC1", "unit": "GB-Hours", "pricePerUnit": {"USD": "0.0001330000"}}
  }}}}
}`

const ecsFargateWindowsOSFeeDoc = `{
  "product": {"sku": "SKU8", "productFamily": "Compute", "attributes": {
    "regionCode": "ap-northeast-1", "usagetype": "APN1-Fargate-Windows-OS-Hours:perCPU", "operatingSystem": "Windows"
  }},
  "terms": {"OnDemand": {"SKU8.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
    "SKU8.OTC1.RC1": {"rateCode": "SKU8.OTC1.RC1", "unit": "hours", "pricePerUnit": {"USD": "0.0460000000"}}
  }}}}
}`

const ecsFargateARMGBDoc = `{
  "product": {"sku": "SKU9", "productFamily": "Compute", "attributes": {
    "regionCode": "ap-northeast-1", "usagetype": "APN1-Fargate-ARM-GB-Hours", "cpuArchitecture": "ARM"
  }},
  "terms": {"OnDemand": {"SKU9.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
    "SKU9.OTC1.RC1": {"rateCode": "SKU9.OTC1.RC1", "unit": "hours", "pricePerUnit": {"USD": "0.0044200000"}}
  }}}}
}`

func TestParsePriceDocument(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		doc, err := parsePriceDocument(ec2PriceDoc)
		if err != nil {
			t.Fatalf("parsePriceDocument() err = %v", err)
		}
		if doc.Product.SKU != "SKU1" {
			t.Errorf("SKU = %q, want SKU1", doc.Product.SKU)
		}
	})
	t.Run("malformed json", func(t *testing.T) {
		if _, err := parsePriceDocument("{not json"); err == nil {
			t.Fatal("parsePriceDocument() err = nil, want error")
		}
	})
	t.Run("missing sku", func(t *testing.T) {
		if _, err := parsePriceDocument(`{"product":{"attributes":{}}}`); err == nil {
			t.Fatal("parsePriceDocument() err = nil, want error for missing sku")
		}
	})
}

func TestPriceRatesFromDocument(t *testing.T) {
	tests := []struct {
		name    string
		service string
		raw     string
		want    []PriceRate
	}{
		{
			name:    "ec2 on-demand and reserved",
			service: "ec2",
			raw:     ec2PriceDoc,
			want: []PriceRate{
				{
					RateID: "SKU1.OTC1.RC1", Model: "on_demand", Group: "On-Demand",
					Label:      "m5.large / Linux / Shared",
					Attributes: map[string]string{"instance_type": "m5.large", "os": "Linux", "tenancy": "Shared"},
					Term:       PriceTerm{},
					Unit:       "Hrs", PriceUSD: 0.124, Currency: "USD",
				},
				{
					RateID: "SKU1.RI1", Model: "reserved", Group: "Reserved Instance",
					Label:      "m5.large / Linux / Shared",
					Attributes: map[string]string{"instance_type": "m5.large", "os": "Linux", "tenancy": "Shared"},
					Term:       PriceTerm{Lease: strPtr("1yr"), OfferingClass: strPtr("standard"), Payment: strPtr("No Upfront")},
					Unit:       "Hrs", PriceUSD: 0.078, Currency: "USD",
				},
				{
					RateID: "SKU1.RI2", Model: "reserved", Group: "Reserved Instance",
					Label:      "m5.large / Linux / Shared",
					Attributes: map[string]string{"instance_type": "m5.large", "os": "Linux", "tenancy": "Shared"},
					Term:       PriceTerm{Lease: strPtr("1yr"), OfferingClass: strPtr("convertible"), Payment: strPtr("All Upfront")},
					Unit:       "Hrs", PriceUSD: 0, UpfrontUSD: 812, Currency: "USD",
				},
			},
		},
		{
			name:    "rds on-demand and reserved",
			service: "rds",
			raw:     rdsPriceDoc,
			want: []PriceRate{
				{
					RateID: "SKU2.OTC1.RC1", Model: "on_demand", Group: "On-Demand",
					Label:      "db.t3.micro / MySQL / Single-AZ",
					Attributes: map[string]string{"instance_type": "db.t3.micro", "engine": "MySQL", "deployment_option": "Single-AZ", "license_model": "No license required"},
					Unit:       "Hrs", PriceUSD: 0.026, Currency: "USD",
				},
				{
					RateID: "SKU2.RI1", Model: "reserved", Group: "Reserved Instance",
					Label:      "db.t3.micro / MySQL / Single-AZ",
					Attributes: map[string]string{"instance_type": "db.t3.micro", "engine": "MySQL", "deployment_option": "Single-AZ", "license_model": "No license required"},
					Term:       PriceTerm{Lease: strPtr("1yr"), OfferingClass: strPtr("standard"), Payment: strPtr("No Upfront")},
					Unit:       "Hrs", PriceUSD: 0.0202, Currency: "USD",
				},
			},
		},
		{
			name:    "elasticache on-demand only",
			service: "elasticache",
			raw:     elastiCachePriceDoc,
			want: []PriceRate{
				{
					RateID: "SKU3.OTC1.RC1", Model: "on_demand", Group: "On-Demand",
					Label:      "cache.t3.micro / Redis",
					Attributes: map[string]string{"instance_type": "cache.t3.micro", "engine": "Redis"},
					Unit:       "Hrs", PriceUSD: 0.026, Currency: "USD",
				},
			},
		},
		{
			name:    "elasticache extended support excluded",
			service: "elasticache",
			raw:     elastiCacheExtendedSupportDoc,
			want:    nil,
		},
		{
			name:    "ecs fargate vcpu",
			service: "ecs",
			raw:     ecsFargateVCPUDoc,
			want: []PriceRate{
				{
					RateID: "SKU5.OTC1.RC1", Model: "on_demand", Group: "On-Demand",
					Label:      "Fargate vCPU / Linux / x86",
					Attributes: map[string]string{"os": "Linux", "architecture": "x86"},
					Unit:       "vCPU-Hours", PriceUSD: 0.05056, Currency: "USD",
				},
			},
		},
		{
			name:    "ecs fargate memory",
			service: "ecs",
			raw:     ecsFargateGBDoc,
			want: []PriceRate{
				{
					RateID: "SKU6.OTC1.RC1", Model: "on_demand", Group: "On-Demand",
					Label:      "Fargate Memory (GB) / Linux / x86",
					Attributes: map[string]string{"os": "Linux", "architecture": "x86"},
					Unit:       "GB-Hours", PriceUSD: 0.00553, Currency: "USD",
				},
			},
		},
		{
			name:    "ecs fargate ephemeral storage excluded",
			service: "ecs",
			raw:     ecsFargateEphemeralDoc,
			want:    nil,
		},
		{
			name:    "ecs fargate windows os license fee excluded (neither vcpu nor memory)",
			service: "ecs",
			raw:     ecsFargateWindowsOSFeeDoc,
			want:    nil,
		},
		{
			name:    "ecs fargate arm memory",
			service: "ecs",
			raw:     ecsFargateARMGBDoc,
			want: []PriceRate{
				{
					RateID: "SKU9.OTC1.RC1", Model: "on_demand", Group: "On-Demand",
					Label:      "Fargate Memory (GB) / Linux / ARM",
					Attributes: map[string]string{"os": "Linux", "architecture": "ARM"},
					Unit:       "GB-Hours", PriceUSD: 0.00442, Currency: "USD",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parsePriceDocument(tt.raw)
			if err != nil {
				t.Fatalf("parsePriceDocument() err = %v", err)
			}
			spec := pricingServiceSpecs[tt.service]
			got := priceRatesFromDocument(tt.service, spec, *doc)
			// doc.Terms.OnDemand/Reserved は map であり Go のイテレーション順序は
			// 非決定的なため (1 ドキュメントが複数の Reserved term を持つ ec2 ケースで
			// 実際に順序違いのフレーキー失敗を確認した)、本番の getPricing が最終的に
			// 適用する sortPriceRates と同じ基準で揃えてから比較する。
			sortPriceRates(got)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("priceRatesFromDocument() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestECSHasNoReservedInstances(t *testing.T) {
	// ECS の spec は riSupported=false であり、Reserved が実データにも一切
	// 現れないことをドキュメントで再現する (仕様上の非対応の裏付け)。
	if pricingServiceSpecs["ecs"].riSupported {
		t.Fatal("ecs spec riSupported = true, want false (Fargate has no RI)")
	}
}

func TestReservedRateFromTerm(t *testing.T) {
	tests := []struct {
		name string
		term priceTermDoc
		ok   bool
	}{
		{
			name: "missing lease is discarded",
			term: priceTermDoc{
				OfferTermCode:  "RI1",
				TermAttributes: priceTermAttributesDoc{PurchaseOption: "No Upfront"},
				PriceDimensions: map[string]priceDimensionDoc{
					"a": {Unit: "Hrs", PricePerUnit: struct {
						USD string `json:"USD"`
					}{USD: "0.1"}},
				},
			},
			ok: false,
		},
		{
			name: "quantity-only term (no hourly dimension) is discarded",
			term: priceTermDoc{
				OfferTermCode: "RI1",
				TermAttributes: priceTermAttributesDoc{
					LeaseContractLength: "1yr", PurchaseOption: "All Upfront",
				},
				PriceDimensions: map[string]priceDimensionDoc{
					"a": {Unit: "Quantity", PricePerUnit: struct {
						USD string `json:"USD"`
					}{USD: "812"}},
				},
			},
			ok: false,
		},
		{
			name: "unparseable price is discarded",
			term: priceTermDoc{
				OfferTermCode: "RI1",
				TermAttributes: priceTermAttributesDoc{
					LeaseContractLength: "1yr", PurchaseOption: "No Upfront",
				},
				PriceDimensions: map[string]priceDimensionDoc{
					"a": {Unit: "Hrs", PricePerUnit: struct {
						USD string `json:"USD"`
					}{USD: "not-a-number"}},
				},
			},
			ok: false,
		},
		{
			name: "no offering class stays nil",
			term: priceTermDoc{
				OfferTermCode: "RI1",
				TermAttributes: priceTermAttributesDoc{
					LeaseContractLength: "1yr", PurchaseOption: "No Upfront",
				},
				PriceDimensions: map[string]priceDimensionDoc{
					"a": {Unit: "Hrs", PricePerUnit: struct {
						USD string `json:"USD"`
					}{USD: "0.02"}},
				},
			},
			ok: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate, ok := reservedRateFromTerm("SKU", tt.term, "label", nil)
			if ok != tt.ok {
				t.Fatalf("reservedRateFromTerm() ok = %v, want %v", ok, tt.ok)
			}
			if ok && tt.name == "no offering class stays nil" && rate.Term.OfferingClass != nil {
				t.Errorf("Term.OfferingClass = %v, want nil", *rate.Term.OfferingClass)
			}
		})
	}
}

func TestFargateUnitFromUsageType(t *testing.T) {
	tests := []struct {
		usageType string
		wantUnit  string
		wantOK    bool
	}{
		{usageType: "APN1-Fargate-vCPU-Hours:perCPU", wantUnit: "vCPU-Hours", wantOK: true},
		{usageType: "APN1-Fargate-GB-Hours", wantUnit: "GB-Hours", wantOK: true},
		{usageType: "APN1-Fargate-Windows-vCPU-Hours:perCPU", wantUnit: "vCPU-Hours", wantOK: true},
		{usageType: "APN1-Fargate-EphemeralStorage-GB-Hours", wantOK: false},
		{usageType: "APN1-Fargate-Windows-OS-Hours:perCPU", wantOK: false},
		{usageType: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.usageType, func(t *testing.T) {
			unit, ok := fargateUnitFromUsageType(tt.usageType)
			if ok != tt.wantOK || unit != tt.wantUnit {
				t.Errorf("fargateUnitFromUsageType(%q) = (%q, %v), want (%q, %v)", tt.usageType, unit, ok, tt.wantUnit, tt.wantOK)
			}
		})
	}
}

func TestLeaseFromDuration(t *testing.T) {
	tests := []struct {
		seconds int64
		want    string
	}{
		{seconds: 31536000, want: "1yr"},
		{seconds: 94608000, want: "3yr"},
		{seconds: 31556952, want: "1yr"}, // 365.2425日換算 (うるう年考慮) でも 1yr に丸まる
	}
	for _, tt := range tests {
		if got := leaseFromDuration(tt.seconds); got != tt.want {
			t.Errorf("leaseFromDuration(%d) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestSpInstanceType(t *testing.T) {
	tests := []struct {
		name      string
		property  string
		usageType string
		want      string
	}{
		{name: "well-formed property", property: "db.m7i.4xlarge", usageType: "APN1-InstanceUsage:db.m7i.4xl", want: "db.m7i.4xlarge"},
		{name: "broken property falls back to usageType suffix", property: "R7G", usageType: "APN1-InstanceUsageIOOptimized:db.r7g.4xl", want: "db.r7g.4xl"},
		{name: "empty property falls back", property: "", usageType: "APN1-BoxUsage:m5.large", want: "m5.large"},
		{name: "both empty", property: "", usageType: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := spInstanceType(tt.property, tt.usageType); got != tt.want {
				t.Errorf("spInstanceType(%q, %q) = %q, want %q", tt.property, tt.usageType, got, tt.want)
			}
		})
	}
}

func TestParseUSD(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  float64
		ok    bool
	}{
		{name: "valid", input: "0.1240000000", want: 0.124, ok: true},
		{name: "integer-like", input: "812", want: 812, ok: true},
		{name: "empty", input: "", want: 0, ok: false},
		{name: "invalid", input: "not-a-number", want: 0, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseUSD(tt.input)
			if ok != tt.ok || got != tt.want {
				t.Errorf("parseUSD(%q) = (%v, %v), want (%v, %v)", tt.input, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func newSPRate(usageType, rate string, offering *sptypes.ParentSavingsPlanOffering, props map[string]string) sptypes.SavingsPlanOfferingRate {
	var properties []sptypes.SavingsPlanOfferingRateProperty
	for k, v := range props {
		k, v := k, v
		properties = append(properties, sptypes.SavingsPlanOfferingRateProperty{Name: &k, Value: &v})
	}
	ut := usageType
	r := rate
	return sptypes.SavingsPlanOfferingRate{
		UsageType:           &ut,
		Rate:                &r,
		Properties:          properties,
		SavingsPlanOffering: offering,
	}
}

func TestInstanceSavingsPlanRate(t *testing.T) {
	offeringID := "offer-1"
	baseOffering := &sptypes.ParentSavingsPlanOffering{
		OfferingId:      &offeringID,
		PaymentOption:   sptypes.SavingsPlanPaymentOptionNoUpfront,
		PlanType:        sptypes.SavingsPlanTypeCompute,
		DurationSeconds: 31536000,
	}

	tests := []struct {
		name    string
		service string
		rate    sptypes.SavingsPlanOfferingRate
		wantOK  bool
	}{
		{
			name:    "plain shared box usage is kept",
			service: "ec2",
			rate: newSPRate("APN1-BoxUsage:m5.large", "0.08", baseOffering, map[string]string{
				"instanceType": "m5.large", "productDescription": "Linux", "region": "ap-northeast-1",
			}),
			wantOK: true,
		},
		{
			name:    "dedicated tenancy excluded",
			service: "ec2",
			rate: newSPRate("APN1-DedicatedUsage:c6i.large", "0.2", baseOffering, map[string]string{
				"instanceType": "c6i.large", "productDescription": "Windows with SQL Server Web",
			}),
			wantOK: false,
		},
		{
			name:    "unused capacity accounting row excluded",
			service: "ec2",
			rate: newSPRate("APN1-UnusedBox:x8i.48xlarge", "22.9", baseOffering, map[string]string{
				"instanceType": "x8i.48xlarge", "productDescription": "Windows",
			}),
			wantOK: false,
		},
		{
			name:    "sql server bundled license excluded",
			service: "rds",
			rate: newSPRate("APN1-InstanceUsage:db.m7i.4xl", "1.58", baseOffering, map[string]string{
				"instanceType": "db.m7i.4xlarge", "productDescription": "SQL Server",
			}),
			wantOK: false,
		},
		{
			name:    "aurora excluded (out of RDS scope)",
			service: "rds",
			rate: newSPRate("APN1-InstanceUsageIOOptimized:db.r7g.4xl", "2.77", baseOffering, map[string]string{
				"instanceType": "R7G", "productDescription": "Aurora MySQL",
			}),
			wantOK: false,
		},
		{
			name:    "elasticache serverless processing units excluded",
			service: "elasticache",
			rate: newSPRate("APN1-ElastiCacheProcessingUnits:Valkey", "0.0000000019", baseOffering, map[string]string{
				"productDescription": "Valkey",
			}),
			wantOK: false,
		},
		{
			name:    "elasticache node usage kept",
			service: "elasticache",
			rate: newSPRate("APN1-NodeUsage:cache.m7g.2xlarge", "0.51", baseOffering, map[string]string{
				"instanceType": "cache.m7g.2xlarge", "productDescription": "Valkey",
			}),
			wantOK: true,
		},
		{
			name:    "nil offering excluded",
			service: "ec2",
			rate:    newSPRate("APN1-BoxUsage:m5.large", "0.08", nil, map[string]string{"instanceType": "m5.large"}),
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usageType := ptrStr(tt.rate.UsageType)
			props := savingsPlanProperties(tt.rate.Properties)
			rate, ok := instanceSavingsPlanRate(tt.service, tt.rate, usageType, props)
			if ok != tt.wantOK {
				t.Fatalf("instanceSavingsPlanRate() ok = %v, want %v (rate=%+v)", ok, tt.wantOK, rate)
			}
			if ok {
				if rate.Model != "savings_plan" {
					t.Errorf("Model = %q, want savings_plan", rate.Model)
				}
				if rate.Term.OfferingClass != nil {
					t.Errorf("Term.OfferingClass = %v, want nil (savings plans have no offering class)", *rate.Term.OfferingClass)
				}
				if rate.UpfrontUSD != 0 {
					t.Errorf("UpfrontUSD = %v, want 0 (SP catalog rates never expose a committed upfront amount)", rate.UpfrontUSD)
				}
			}
		})
	}

	t.Run("broken instanceType property falls back to usageType", func(t *testing.T) {
		rate := newSPRate("APN1-InstanceUsageIOOptimized:db.r7g.4xl", "2.77", baseOffering, map[string]string{
			"instanceType": "R7G", "productDescription": "MySQL",
		})
		got, ok := instanceSavingsPlanRate("rds", rate, ptrStr(rate.UsageType), savingsPlanProperties(rate.Properties))
		if !ok {
			t.Fatal("instanceSavingsPlanRate() ok = false, want true")
		}
		if got.Attributes["instance_type"] != "db.r7g.4xl" {
			t.Errorf("Attributes[instance_type] = %q, want %q", got.Attributes["instance_type"], "db.r7g.4xl")
		}
	})
}

func TestEcsSavingsPlanRate(t *testing.T) {
	offeringID := "offer-ecs"
	offering := &sptypes.ParentSavingsPlanOffering{
		OfferingId:      &offeringID,
		PaymentOption:   sptypes.SavingsPlanPaymentOptionPartialUpfront,
		PlanType:        sptypes.SavingsPlanTypeCompute,
		DurationSeconds: 94608000,
	}

	tests := []struct {
		name      string
		usageType string
		wantOK    bool
		wantLabel string
	}{
		{name: "vcpu linux x86", usageType: "APN1-Fargate-vCPU-Hours:perCPU", wantOK: true, wantLabel: "Fargate vCPU / Linux / x86"},
		{name: "memory windows", usageType: "APN1-Fargate-Windows-GB-Hours", wantOK: true, wantLabel: "Fargate Memory (GB) / Windows / x86"},
		{name: "vcpu arm", usageType: "APN1-Fargate-ARM-vCPU-Hours:perCPU", wantOK: true, wantLabel: "Fargate vCPU / Linux / ARM"},
		{name: "ephemeral storage excluded", usageType: "APN1-Fargate-EphemeralStorage-GB-Hours", wantOK: false},
		{name: "windows os license fee excluded", usageType: "APN1-Fargate-Windows-OS-Hours:perCPU", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate := newSPRate(tt.usageType, "0.03", offering, map[string]string{"tenancy": "shared", "region": "ap-northeast-1"})
			got, ok := ecsSavingsPlanRate(rate, tt.usageType)
			if ok != tt.wantOK {
				t.Fatalf("ecsSavingsPlanRate() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got.Label != tt.wantLabel {
				t.Errorf("Label = %q, want %q", got.Label, tt.wantLabel)
			}
		})
	}
}

// ---- fakes for orchestration tests ----

type fakePricingClient struct {
	getProducts func(*pricing.GetProductsInput) (*pricing.GetProductsOutput, error)
}

func (f *fakePricingClient) GetProducts(_ context.Context, p *pricing.GetProductsInput, _ ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
	return f.getProducts(p)
}

type fakeSavingsPlansClient struct {
	describe func(*savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error)
}

func (f *fakeSavingsPlansClient) DescribeSavingsPlansOfferingRates(_ context.Context, p *savingsplans.DescribeSavingsPlansOfferingRatesInput, _ ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
	return f.describe(p)
}

func TestFetchOnDemandAndReservedPagination(t *testing.T) {
	calls := 0
	client := &fakePricingClient{
		getProducts: func(in *pricing.GetProductsInput) (*pricing.GetProductsOutput, error) {
			calls++
			if in.NextToken == nil {
				token := "page2"
				return &pricing.GetProductsOutput{PriceList: []string{ec2PriceDoc}, NextToken: &token}, nil
			}
			return &pricing.GetProductsOutput{PriceList: []string{ec2PriceDoc}}, nil
		},
	}
	rates, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "ec2", pricingServiceSpecs["ec2"])
	if err != nil {
		t.Fatalf("fetchOnDemandAndReserved() err = %v", err)
	}
	if calls != 2 {
		t.Errorf("GetProducts called %d times, want 2 (pagination)", calls)
	}
	// 両ページとも ec2PriceDoc (productFamily: Compute Instance) を返すため、
	// priceRatesFromDocument の productFamily チェックを通過して両方が合算される。
	if len(rates) != 6 {
		t.Fatalf("fetchOnDemandAndReserved() returned %d rates, want 6 (3 rates/page x 2 pages)", len(rates))
	}
}

// TestFetchOnDemandAndReservedStopsOnEmptyStringNextToken は
// TestFetchSavingsPlansPaginationStopsOnEmptyStringNextToken と対になる回帰
// テスト。GetProducts では実データ上まだ観測していないが、DescribeSavingsPlans
// OfferingRates と同じ *string 型の NextToken を使う AWS API 一般の防御として、
// GetProducts 側でも空文字列を終端として扱えることを確認する。
func TestFetchOnDemandAndReservedStopsOnEmptyStringNextToken(t *testing.T) {
	calls := 0
	client := &fakePricingClient{
		getProducts: func(in *pricing.GetProductsInput) (*pricing.GetProductsOutput, error) {
			calls++
			if calls > 1 {
				return nil, errors.New("unexpected second call")
			}
			emptyToken := ""
			return &pricing.GetProductsOutput{PriceList: []string{ec2PriceDoc}, NextToken: &emptyToken}, nil
		},
	}
	rates, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "ec2", pricingServiceSpecs["ec2"])
	if err != nil {
		t.Fatalf("fetchOnDemandAndReserved() err = %v, want nil (empty-string NextToken must be treated as no more pages)", err)
	}
	if calls != 1 {
		t.Errorf("GetProducts called %d times, want 1 (empty-string NextToken must stop pagination)", calls)
	}
	if len(rates) != 3 {
		t.Errorf("len(rates) = %d, want 3", len(rates))
	}
}

func TestFetchOnDemandAndReservedErrorAbortsWithoutPartialData(t *testing.T) {
	client := &fakePricingClient{
		getProducts: func(*pricing.GetProductsInput) (*pricing.GetProductsOutput, error) {
			return nil, errors.New("throttled")
		},
	}
	rates, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "ec2", pricingServiceSpecs["ec2"])
	if err == nil {
		t.Fatal("fetchOnDemandAndReserved() err = nil, want error")
	}
	if rates != nil {
		t.Errorf("fetchOnDemandAndReserved() rates = %v, want nil on error", rates)
	}
}

func TestFetchOnDemandAndReservedSkipsMalformedEntries(t *testing.T) {
	client := &fakePricingClient{
		getProducts: func(*pricing.GetProductsInput) (*pricing.GetProductsOutput, error) {
			return &pricing.GetProductsOutput{PriceList: []string{"{not json", ec2PriceDoc}}, nil
		},
	}
	rates, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "ec2", pricingServiceSpecs["ec2"])
	if err != nil {
		t.Fatalf("fetchOnDemandAndReserved() err = %v, want nil (malformed entries are skipped, not fatal)", err)
	}
	if len(rates) == 0 {
		t.Fatal("fetchOnDemandAndReserved() returned no rates; the valid entry should still have been parsed")
	}
}

func TestFetchOnDemandAndReservedEC2Filters(t *testing.T) {
	var captured *pricing.GetProductsInput
	client := &fakePricingClient{
		getProducts: func(in *pricing.GetProductsInput) (*pricing.GetProductsOutput, error) {
			captured = in
			return &pricing.GetProductsOutput{}, nil
		},
	}
	if _, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "ec2", pricingServiceSpecs["ec2"]); err != nil {
		t.Fatalf("fetchOnDemandAndReserved() err = %v", err)
	}
	if len(captured.Filters) != 5 {
		t.Fatalf("ec2 filters count = %d, want 5 (regionCode + productFamily + tenancy + capacitystatus + preInstalledSw)", len(captured.Filters))
	}
}

func TestFetchOnDemandAndReservedRDSFilters(t *testing.T) {
	var captured *pricing.GetProductsInput
	client := &fakePricingClient{
		getProducts: func(in *pricing.GetProductsInput) (*pricing.GetProductsOutput, error) {
			captured = in
			return &pricing.GetProductsOutput{}, nil
		},
	}
	if _, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "rds", pricingServiceSpecs["rds"]); err != nil {
		t.Fatalf("fetchOnDemandAndReserved() err = %v", err)
	}
	if len(captured.Filters) != 2 {
		t.Fatalf("rds filters count = %d, want 2 (regionCode + productFamily)", len(captured.Filters))
	}
}

func TestFetchSavingsPlansPagination(t *testing.T) {
	calls := 0
	client := &fakeSavingsPlansClient{
		describe: func(in *savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			calls++
			rate := newSPRate("APN1-BoxUsage:m5.large", "0.08", &sptypes.ParentSavingsPlanOffering{
				OfferingId: strPtr("o1"), PaymentOption: sptypes.SavingsPlanPaymentOptionNoUpfront,
				PlanType: sptypes.SavingsPlanTypeCompute, DurationSeconds: 31536000,
			}, map[string]string{"instanceType": "m5.large", "productDescription": "Linux"})
			if in.NextToken == nil {
				token := "page2"
				return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{SearchResults: []sptypes.SavingsPlanOfferingRate{rate}, NextToken: &token}, nil
			}
			return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{SearchResults: []sptypes.SavingsPlanOfferingRate{rate}}, nil
		},
	}
	rates, err := fetchSavingsPlans(context.Background(), client, "ap-northeast-1", "ec2", pricingServiceSpecs["ec2"])
	if err != nil {
		t.Fatalf("fetchSavingsPlans() err = %v", err)
	}
	if calls != 2 {
		t.Errorf("DescribeSavingsPlansOfferingRates called %d times, want 2 (pagination)", calls)
	}
	if len(rates) != 2 {
		t.Errorf("len(rates) = %d, want 2", len(rates))
	}
}

// TestFetchSavingsPlansPaginationStopsOnEmptyStringNextToken は、実際の
// DescribeSavingsPlansOfferingRates (RDS/ElastiCache の Database SP のように
// 該当件数が少なく 1 ページで完結するケース) が最終ページで NextToken を nil
// ではなく空文字列で返すことを実データで確認した回帰テスト。空文字列を次ページ
// ありと誤認してリクエストすると、AWS 側が NextToken の正規表現バリデーション
// で 400 ValidationException を返し、SP 取得全体が失敗する。
func TestFetchSavingsPlansPaginationStopsOnEmptyStringNextToken(t *testing.T) {
	calls := 0
	client := &fakeSavingsPlansClient{
		describe: func(in *savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			calls++
			if calls > 1 {
				return nil, errors.New("ValidationException: NextToken must satisfy regex (unexpected second call)")
			}
			rate := newSPRate("APN1-InstanceUsage:db.m7g.12xl", "4.5", &sptypes.ParentSavingsPlanOffering{
				OfferingId: strPtr("o1"), PaymentOption: sptypes.SavingsPlanPaymentOptionNoUpfront,
				PlanType: sptypes.SavingsPlanTypeDatabase, DurationSeconds: 31536000,
			}, map[string]string{"instanceType": "db.m7g.12xl", "productDescription": "MariaDB"})
			emptyToken := ""
			return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
				SearchResults: []sptypes.SavingsPlanOfferingRate{rate},
				NextToken:     &emptyToken,
			}, nil
		},
	}
	rates, err := fetchSavingsPlans(context.Background(), client, "ap-northeast-1", "rds", pricingServiceSpecs["rds"])
	if err != nil {
		t.Fatalf("fetchSavingsPlans() err = %v, want nil (empty-string NextToken must be treated as no more pages)", err)
	}
	if calls != 1 {
		t.Errorf("DescribeSavingsPlansOfferingRates called %d times, want 1 (empty-string NextToken must stop pagination)", calls)
	}
	if len(rates) != 1 {
		t.Errorf("len(rates) = %d, want 1", len(rates))
	}
}

func TestFetchSavingsPlansError(t *testing.T) {
	client := &fakeSavingsPlansClient{
		describe: func(*savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			return nil, errors.New("access denied")
		},
	}
	if _, err := fetchSavingsPlans(context.Background(), client, "ap-northeast-1", "ec2", pricingServiceSpecs["ec2"]); err == nil {
		t.Fatal("fetchSavingsPlans() err = nil, want error")
	}
}

func TestGetPricingOrchestration(t *testing.T) {
	okPricing := &fakePricingClient{
		getProducts: func(*pricing.GetProductsInput) (*pricing.GetProductsOutput, error) {
			return &pricing.GetProductsOutput{PriceList: []string{ec2PriceDoc}}, nil
		},
	}
	failPricing := &fakePricingClient{
		getProducts: func(*pricing.GetProductsInput) (*pricing.GetProductsOutput, error) {
			return nil, errors.New("boom")
		},
	}
	okSP := &fakeSavingsPlansClient{
		describe: func(*savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			rate := newSPRate("APN1-BoxUsage:m5.large", "0.08", &sptypes.ParentSavingsPlanOffering{
				OfferingId: strPtr("o1"), PaymentOption: sptypes.SavingsPlanPaymentOptionNoUpfront,
				PlanType: sptypes.SavingsPlanTypeCompute, DurationSeconds: 31536000,
			}, map[string]string{"instanceType": "m5.large", "productDescription": "Linux"})
			return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{SearchResults: []sptypes.SavingsPlanOfferingRate{rate}}, nil
		},
	}
	failSP := &fakeSavingsPlansClient{
		describe: func(*savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			return nil, errors.New("sp unavailable")
		},
	}

	t.Run("both succeed", func(t *testing.T) {
		table, err := getPricing(context.Background(), okPricing, okSP, "ap-northeast-1", "ec2", pricingServiceSpecs["ec2"])
		if err != nil {
			t.Fatalf("getPricing() err = %v", err)
		}
		if table.Partial {
			t.Error("Partial = true, want false")
		}
		if len(table.MissingModels) != 0 {
			t.Errorf("MissingModels = %v, want empty", table.MissingModels)
		}
		hasSP := false
		for _, r := range table.Rates {
			if r.Model == "savings_plan" {
				hasSP = true
			}
		}
		if !hasSP {
			t.Error("expected a savings_plan rate to be present")
		}
	})

	t.Run("savings plans failure degrades to partial", func(t *testing.T) {
		table, err := getPricing(context.Background(), okPricing, failSP, "ap-northeast-1", "ec2", pricingServiceSpecs["ec2"])
		if err != nil {
			t.Fatalf("getPricing() err = %v, want nil (SP failure must degrade, not error)", err)
		}
		if !table.Partial {
			t.Error("Partial = false, want true")
		}
		if diff := cmp.Diff([]string{"savings_plan"}, table.MissingModels); diff != "" {
			t.Errorf("MissingModels mismatch (-want +got):\n%s", diff)
		}
		for _, r := range table.Rates {
			if r.Model == "savings_plan" {
				t.Error("Rates contains a savings_plan entry despite SP fetch failure")
			}
		}
	})

	t.Run("on-demand failure aborts entirely", func(t *testing.T) {
		_, err := getPricing(context.Background(), failPricing, okSP, "ap-northeast-1", "ec2", pricingServiceSpecs["ec2"])
		if err == nil {
			t.Fatal("getPricing() err = nil, want error (on-demand/RI failure must not be cached as partial)")
		}
	})
}

func TestValidatePricingService(t *testing.T) {
	tests := []struct {
		service string
		wantErr bool
	}{
		{service: "ec2", wantErr: false},
		{service: "rds", wantErr: false},
		{service: "elasticache", wantErr: false},
		{service: "ecs", wantErr: false},
		{service: "s3", wantErr: true},
		{service: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			err := ValidatePricingService(tt.service)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePricingService(%q) err = %v, wantErr %v", tt.service, err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidPricingService) {
				t.Errorf("ValidatePricingService(%q) err = %v, want wrapping ErrInvalidPricingService", tt.service, err)
			}
		})
	}
}
