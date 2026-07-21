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

// 実際に AWS Price List API から取得した生 JSON (issue 0053 実装時にライブ検証済み: EC2
// Windows は operation/licenseModel 属性を持ち、"RunInstances:0002" は "No License required"
// に対応する) をもとにしたテスト用ドキュメント。
const ec2WindowsPriceDoc = `{
  "product": {"sku": "SKU12", "productFamily": "Compute Instance", "attributes": {
    "instanceType": "m5.large", "operatingSystem": "Windows", "tenancy": "Shared",
    "licenseModel": "No License required", "operation": "RunInstances:0002",
    "regionCode": "ap-northeast-1", "usagetype": "APN1-BoxUsage:m5.large",
    "capacitystatus": "Used", "preInstalledSw": "NA"
  }},
  "terms": {
    "OnDemand": {"SKU12.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
      "SKU12.OTC1.RC1": {"rateCode": "SKU12.OTC1.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "0.2320000000"}}
    }}}
  }
}`

const rdsPriceDoc = `{
  "product": {"sku": "SKU2", "productFamily": "Database Instance", "attributes": {
    "instanceType": "db.t3.micro", "databaseEngine": "MySQL", "deploymentOption": "Single-AZ",
    "storage": "EBS Only", "licenseModel": "No license required",
    "regionCode": "ap-northeast-1", "usagetype": "APN1-InstanceUsage:db.t3.micro"
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

const rdsAuroraPriceDoc = `{
  "product": {"sku": "SKU10", "productFamily": "Database Instance", "attributes": {
    "instanceType": "db.r6g.large", "databaseEngine": "Aurora MySQL", "deploymentOption": "Single-AZ",
    "storage": "EBS Only", "licenseModel": "No license required",
    "regionCode": "ap-northeast-1", "usagetype": "APN1-InstanceUsage:db.r6g.large"
  }},
  "terms": {
    "OnDemand": {"SKU10.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
      "SKU10.OTC1.RC1": {"rateCode": "SKU10.OTC1.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "0.2200000000"}}
    }}}
  }
}`

const rdsAuroraIOOptimizedPriceDoc = `{
  "product": {"sku": "SKU11", "productFamily": "Database Instance", "attributes": {
    "instanceType": "db.r6g.large", "databaseEngine": "Aurora MySQL", "deploymentOption": "Single-AZ",
    "storage": "Aurora IO Optimization Mode", "licenseModel": "No license required",
    "regionCode": "ap-northeast-1", "usagetype": "APN1-InstanceUsageIOOptimized:db.r6g.large"
  }},
  "terms": {
    "OnDemand": {"SKU11.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
      "SKU11.OTC1.RC1": {"rateCode": "SKU11.OTC1.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "0.2860000000"}}
    }}}
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
					Attributes: map[string]string{"instance_type": "m5.large", "instance_family": "m5", "os": "Linux", "tenancy": "Shared"},
					Term:       PriceTerm{},
					Unit:       "Hrs", PriceUSD: 0.124, Currency: "USD",
				},
				{
					RateID: "SKU1.RI1", Model: "reserved", Group: "Reserved Instance",
					Label:      "m5.large / Linux / Shared",
					Attributes: map[string]string{"instance_type": "m5.large", "instance_family": "m5", "os": "Linux", "tenancy": "Shared"},
					Term:       PriceTerm{Lease: strPtr("1yr"), OfferingClass: strPtr("standard"), Payment: strPtr("No Upfront")},
					Unit:       "Hrs", PriceUSD: 0.078, Currency: "USD",
				},
				{
					RateID: "SKU1.RI2", Model: "reserved", Group: "Reserved Instance",
					Label:      "m5.large / Linux / Shared",
					Attributes: map[string]string{"instance_type": "m5.large", "instance_family": "m5", "os": "Linux", "tenancy": "Shared"},
					Term:       PriceTerm{Lease: strPtr("1yr"), OfferingClass: strPtr("convertible"), Payment: strPtr("All Upfront")},
					Unit:       "Hrs", PriceUSD: 0, UpfrontUSD: 812, Currency: "USD",
				},
			},
		},
		{
			// issue 0053: EC2 の licenseModel/operation は curatedInstanceAttributes/
			// instanceLabel に反映される (RDS の license_model と同じ扱い)。
			name:    "ec2 windows with license model",
			service: "ec2",
			raw:     ec2WindowsPriceDoc,
			want: []PriceRate{
				{
					RateID: "SKU12.OTC1.RC1", Model: "on_demand", Group: "On-Demand",
					Label:      "m5.large / Windows / Shared / No License required",
					Attributes: map[string]string{"instance_type": "m5.large", "instance_family": "m5", "os": "Windows", "tenancy": "Shared", "license_model": "No License required"},
					Term:       PriceTerm{},
					Unit:       "Hrs", PriceUSD: 0.232, Currency: "USD",
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
					Label:      "db.t3.micro / MySQL / Single-AZ / Standard / No license required",
					Attributes: map[string]string{"instance_type": "db.t3.micro", "instance_family": "db.t3", "engine": "MySQL", "deployment_option": "Single-AZ", "license_model": "No license required", "storage_type": "standard"},
					Unit:       "Hrs", PriceUSD: 0.026, Currency: "USD",
				},
				{
					RateID: "SKU2.RI1", Model: "reserved", Group: "Reserved Instance",
					Label:      "db.t3.micro / MySQL / Single-AZ / Standard / No license required",
					Attributes: map[string]string{"instance_type": "db.t3.micro", "instance_family": "db.t3", "engine": "MySQL", "deployment_option": "Single-AZ", "license_model": "No license required", "storage_type": "standard"},
					Term:       PriceTerm{Lease: strPtr("1yr"), OfferingClass: strPtr("standard"), Payment: strPtr("No Upfront")},
					Unit:       "Hrs", PriceUSD: 0.0202, Currency: "USD",
				},
			},
		},
		{
			// Aurora は RDS スコープに含める (実際に Aurora MySQL/PostgreSQL を運用して
			// いるユーザーからのフィードバックにより、非 Aurora 限定のスコープ案は不採用)。
			// storage 属性が "EBS Only" (Aurora の既定ストレージ) の行は
			// storage_type: "standard" になる。
			name:    "rds aurora standard storage",
			service: "rds",
			raw:     rdsAuroraPriceDoc,
			want: []PriceRate{
				{
					RateID: "SKU10.OTC1.RC1", Model: "on_demand", Group: "On-Demand",
					Label:      "db.r6g.large / Aurora MySQL / Single-AZ / Standard / No license required",
					Attributes: map[string]string{"instance_type": "db.r6g.large", "instance_family": "db.r6g", "engine": "Aurora MySQL", "deployment_option": "Single-AZ", "license_model": "No license required", "storage_type": "standard"},
					Unit:       "Hrs", PriceUSD: 0.22, Currency: "USD",
				},
			},
		},
		{
			// IO-Optimized ストレージは Aurora 専用機能で、storage 属性が正確に
			// "Aurora IO Optimization Mode" の行だけが storage_type: "io_optimized"
			// になる (実データ確認済み、非 Aurora RDS にはこの storage 値は存在しない)。
			name:    "rds aurora io-optimized storage",
			service: "rds",
			raw:     rdsAuroraIOOptimizedPriceDoc,
			want: []PriceRate{
				{
					RateID: "SKU11.OTC1.RC1", Model: "on_demand", Group: "On-Demand",
					Label:      "db.r6g.large / Aurora MySQL / Single-AZ / IO-Optimized / No license required",
					Attributes: map[string]string{"instance_type": "db.r6g.large", "instance_family": "db.r6g", "engine": "Aurora MySQL", "deployment_option": "Single-AZ", "license_model": "No license required", "storage_type": "io_optimized"},
					Unit:       "Hrs", PriceUSD: 0.286, Currency: "USD",
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
					Attributes: map[string]string{"instance_type": "cache.t3.micro", "instance_family": "cache.t3", "engine": "Redis"},
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
			spec := resourceServiceSpecs[tt.service]
			got := priceRatesFromDocument(tt.service, spec, *doc, map[string]string{})
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

// issue 0053: Savings Plans の Properties にはライセンスモデル情報が無いため、On-Demand/
// Reserved の生属性から operation→licenseModel の対応表を副産物として組み立てる。
func TestRecordOperationLicenseModel(t *testing.T) {
	t.Run("records operation to license model", func(t *testing.T) {
		dest := map[string]string{}
		recordOperationLicenseModel(dest, map[string]string{
			"operation": "CreateDBInstance:0020", "licenseModel": "License included",
		})
		if dest["CreateDBInstance:0020"] != "License included" {
			t.Errorf(`dest["CreateDBInstance:0020"] = %q, want "License included"`, dest["CreateDBInstance:0020"])
		}
	})

	t.Run("missing operation or licenseModel is ignored", func(t *testing.T) {
		dest := map[string]string{}
		recordOperationLicenseModel(dest, map[string]string{"operation": "CreateDBInstance:0020"})
		recordOperationLicenseModel(dest, map[string]string{"licenseModel": "License included"})
		recordOperationLicenseModel(dest, map[string]string{})
		if len(dest) != 0 {
			t.Errorf("dest = %v, want empty", dest)
		}
	})

	t.Run("conflicting license model for the same operation keeps the first-seen value", func(t *testing.T) {
		dest := map[string]string{}
		recordOperationLicenseModel(dest, map[string]string{
			"operation": "CreateDBInstance:0020", "licenseModel": "License included",
		})
		recordOperationLicenseModel(dest, map[string]string{
			"operation": "CreateDBInstance:0020", "licenseModel": "Bring your own license",
		})
		if dest["CreateDBInstance:0020"] != "License included" {
			t.Errorf(`dest["CreateDBInstance:0020"] = %q, want first-seen value "License included"`, dest["CreateDBInstance:0020"])
		}
	})
}

func TestECSHasNoReservedInstances(t *testing.T) {
	// ECS の spec は riSupported=false であり、Reserved が実データにも一切
	// 現れないことをドキュメントで再現する (仕様上の非対応の裏付け)。
	if resourceServiceSpecs["ecs"].riSupported {
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

// issue 0054: ファミリはインスタンスタイプから末尾のサイズトークンを除いた文字列とする。
// spInstanceType が返す usageType 由来の省略形 (末尾サイズが短縮されていてもドット区切りは
// 保たれる) でも、On-Demand/RI と同じ規則でファミリへ正規化できることを確認する。
func TestInstanceFamily(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		want         string
	}{
		{name: "ec2", instanceType: "m5.large", want: "m5"},
		{name: "rds", instanceType: "db.r6g.4xlarge", want: "db.r6g"},
		{name: "elasticache", instanceType: "cache.t4g.micro", want: "cache.t4g"},
		{name: "savings plans abbreviated size (spInstanceType usageType fallback)", instanceType: "db.r7g.4xl", want: "db.r7g"},
		{name: "empty", instanceType: "", want: ""},
		{name: "no dot (malformed, returned unchanged)", instanceType: "R7G", want: "R7G"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := instanceFamily(tt.instanceType); got != tt.want {
				t.Errorf("instanceFamily(%q) = %q, want %q", tt.instanceType, got, tt.want)
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
			// Aurora は RDS スコープに含まれる。Savings Plans の Aurora 行は
			// instanceType プロパティが壊れている (instance family のみの大文字表記)
			// ことがあるが、それは spInstanceType の usageType フォールバックで
			// 別途対処済み (下の "broken instanceType property" サブテスト参照)。
			name:    "aurora kept (in RDS scope)",
			service: "rds",
			rate: newSPRate("APN1-InstanceUsageIOOptimized:db.r7g.4xl", "2.77", baseOffering, map[string]string{
				"instanceType": "R7G", "productDescription": "Aurora MySQL",
			}),
			wantOK: true,
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
		// instance_family は On-Demand/RI と同じファミリ表記 (db.r7g) へ正規化される
		// (usageType 由来の省略形でもドット区切りは保たれるため instanceFamily がそのまま使える)。
		if got.Attributes["instance_family"] != "db.r7g" {
			t.Errorf("Attributes[instance_family] = %q, want %q", got.Attributes["instance_family"], "db.r7g")
		}
	})

	// issue 0049: 同じ offeringId + usageType (= 同じ instance_type) でも、Savings Plans は
	// 特定の engine/OS に縛られないため productDescription が異なる複数行を返す。RateID に
	// productDescription を含めない実装だと、これらが同一の RateID を持ってしまい React の
	// key 重複と選択状態 (rate_id をキーにした Record) の誤連動を引き起こす。
	t.Run("same offering+usageType with different productDescription yields distinct RateID", func(t *testing.T) {
		mysqlRate := newSPRate("APN1-InstanceUsage:db.m7g.12xl", "1.0", baseOffering, map[string]string{
			"instanceType": "db.m7g.12xlarge", "productDescription": "MySQL",
		})
		mariaRate := newSPRate("APN1-InstanceUsage:db.m7g.12xl", "1.0", baseOffering, map[string]string{
			"instanceType": "db.m7g.12xlarge", "productDescription": "MariaDB",
		})
		got1, ok1 := instanceSavingsPlanRate("rds", mysqlRate, ptrStr(mysqlRate.UsageType), savingsPlanProperties(mysqlRate.Properties))
		got2, ok2 := instanceSavingsPlanRate("rds", mariaRate, ptrStr(mariaRate.UsageType), savingsPlanProperties(mariaRate.Properties))
		if !ok1 || !ok2 {
			t.Fatalf("instanceSavingsPlanRate() ok1 = %v, ok2 = %v, want both true", ok1, ok2)
		}
		if got1.RateID == got2.RateID {
			t.Errorf("RateID collision: MySQL and MariaDB both got %q, want distinct RateIDs", got1.RateID)
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

// issue 0052: DescribeSavingsPlansOfferingRates は同一レートに対して、内部的な Operation
// コードのみが異なる (Rate や Properties は完全一致する) 複数の SearchResults エントリを
// 返すことがある (RDS の Oracle/Db2 で実データ確認済み)。Operation は PriceRate のどの
// フィールドにも取り込まれないため、そこから生成される PriceRate は完全に同一の内容になる。
func TestDedupeSavingsPlanRates(t *testing.T) {
	rate := func(priceUSD float64) PriceRate {
		return PriceRate{
			RateID:     "offer-1#APN1-InstanceUsage:db.m7i.12xl#Db2",
			Model:      "savings_plan",
			Group:      "Database Savings Plans",
			Label:      "db.m7i.12xl / Db2",
			Attributes: map[string]string{"engine": "Db2", "instance_type": "db.m7i.12xl"},
			Term:       PriceTerm{Lease: strPtr("1yr"), Payment: strPtr("No Upfront")},
			Unit:       "Hrs",
			PriceUSD:   priceUSD,
			Currency:   "USD",
		}
	}

	t.Run("完全に同一の行 (Operation 違いのみ) は 1 件にまとまる", func(t *testing.T) {
		got := dedupeSavingsPlanRates([]PriceRate{rate(4.7424), rate(4.7424), rate(4.7424)})
		if len(got) != 1 {
			t.Fatalf("len(got) = %d, want 1", len(got))
		}
	})

	t.Run("RateID が同じでも価格が異なれば別行として残す (誤統合しない)", func(t *testing.T) {
		got := dedupeSavingsPlanRates([]PriceRate{rate(4.7424), rate(5.0)})
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2 (誤って統合された)", len(got))
		}
	})

	t.Run("互いに異なる複数行はすべて残る", func(t *testing.T) {
		r1 := rate(4.7424)
		r2 := rate(4.7424)
		r2.RateID = "offer-1#APN1-InstanceUsage:db.m7i.16xl#Db2"
		r2.Label = "db.m7i.16xl / Db2"
		got := dedupeSavingsPlanRates([]PriceRate{r1, r2})
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}
	})

	t.Run("空スライスは空スライスのまま", func(t *testing.T) {
		got := dedupeSavingsPlanRates([]PriceRate{})
		if len(got) != 0 {
			t.Fatalf("len(got) = %d, want 0", len(got))
		}
	})
}

// TestFetchSavingsPlansDedupesOperationVariants は issue 0052 の実データ形状 (同一
// OfferingId/UsageType/Properties/Rate だが Operation のみ異なる 2 エントリ) を
// DescribeSavingsPlansOfferingRates のフェイク応答として与え、fetchSavingsPlans が返す
// PriceRate が 1 件に統合されることを検証する。
func TestFetchSavingsPlansDedupesOperationVariants(t *testing.T) {
	offeringID := "8870e805-fb04-4245-820e-231a54e5121b"
	offering := &sptypes.ParentSavingsPlanOffering{
		OfferingId:      &offeringID,
		PaymentOption:   sptypes.SavingsPlanPaymentOptionNoUpfront,
		PlanType:        sptypes.SavingsPlanTypeDatabase,
		DurationSeconds: 31536000,
	}
	props := map[string]string{"instanceType": "M7i", "productDescription": "Db2", "region": "ap-northeast-1"}
	// Operation は SavingsPlanOfferingRate のフィールドだが savingsPlanRateFrom はこれを
	// 読まないため、フェイク側でも設定不要 (実データでの差異点そのものを再現する必要はなく、
	// 「PriceRate に変換されると区別がつかなくなる 2 エントリ」であれば十分)。
	rate1 := newSPRate("APN1-InstanceUsage:db.m7i.12xl", "4.7424000000", offering, props)
	rate1.ServiceCode = sptypes.SavingsPlanRateServiceCodeRds
	rate2 := newSPRate("APN1-InstanceUsage:db.m7i.12xl", "4.7424000000", offering, props)
	rate2.ServiceCode = sptypes.SavingsPlanRateServiceCodeRds

	client := &fakeSavingsPlansClient{
		describe: func(*savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
				SearchResults: []sptypes.SavingsPlanOfferingRate{rate1, rate2},
			}, nil
		},
	}

	got, err := fetchSavingsPlans(context.Background(), client, "ap-northeast-1", savingsPlanServiceSpecs["database-sp"])
	if err != nil {
		t.Fatalf("fetchSavingsPlans() err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("fetchSavingsPlans() returned %d rates, want 1 (Operation 違いの重複が残っている)", len(got))
	}
}

// issue 0053: Savings Plans の Properties にはライセンスモデル情報が無いため (RDS Oracle の
// BYOL/License Included、EC2 Windows の BYOL/標準ライセンスはいずれも offeringId+usageType+
// productDescription が完全一致するのに price_usd が本当に異なる)、Operation コード経由で
// On-Demand 側の対応表 (recordOperationLicenseModel が作る) から逆引きする。
func TestApplySavingsPlanLicenseModel(t *testing.T) {
	t.Run("resolvable operation gets license_model appended to RateID/Label/Attributes", func(t *testing.T) {
		rates := []PriceRate{{
			RateID:     "offer-1#APN1-InstanceUsage:db.m8i.2xl#Oracle",
			Label:      "db.m8i.2xlarge / Oracle",
			Attributes: map[string]string{"instance_type": "db.m8i.2xlarge", "engine": "Oracle"},
			Operation:  "CreateDBInstance:0005",
		}}
		opLicense := map[string]string{"CreateDBInstance:0005": "Bring your own license"}

		got := applySavingsPlanLicenseModel(rates, opLicense)

		wantRateID := "offer-1#APN1-InstanceUsage:db.m8i.2xl#Oracle#Bring your own license"
		if got[0].RateID != wantRateID {
			t.Errorf("RateID = %q, want %q", got[0].RateID, wantRateID)
		}
		wantLabel := "db.m8i.2xlarge / Oracle / Bring your own license"
		if got[0].Label != wantLabel {
			t.Errorf("Label = %q, want %q", got[0].Label, wantLabel)
		}
		if got[0].Attributes["license_model"] != "Bring your own license" {
			t.Errorf(`Attributes["license_model"] = %q, want "Bring your own license"`, got[0].Attributes["license_model"])
		}
	})

	t.Run("unresolvable operation is left unchanged", func(t *testing.T) {
		rates := []PriceRate{{
			RateID:     "offer-1#APN1-InstanceUsage:db.t3.micro#MySQL",
			Label:      "db.t3.micro / MySQL",
			Attributes: map[string]string{"instance_type": "db.t3.micro", "engine": "MySQL"},
			Operation:  "CreateDBInstance:0002",
		}}

		got := applySavingsPlanLicenseModel(rates, map[string]string{})

		if got[0].RateID != "offer-1#APN1-InstanceUsage:db.t3.micro#MySQL" {
			t.Errorf("RateID changed unexpectedly: %q", got[0].RateID)
		}
		if got[0].Label != "db.t3.micro / MySQL" {
			t.Errorf("Label changed unexpectedly: %q", got[0].Label)
		}
		if _, ok := got[0].Attributes["license_model"]; ok {
			t.Errorf(`Attributes["license_model"] set unexpectedly: %q`, got[0].Attributes["license_model"])
		}
	})

	t.Run("two colliding rows resolve to distinct RateID/Label (issue 0053 reproduction)", func(t *testing.T) {
		licenseIncluded := PriceRate{
			RateID:     "offer-1#APN1-InstanceUsage:db.m8i.2xl#Oracle",
			Label:      "db.m8i.2xlarge / Oracle",
			Attributes: map[string]string{"instance_type": "db.m8i.2xlarge", "engine": "Oracle"},
			PriceUSD:   1.6448,
			Operation:  "CreateDBInstance:0020",
		}
		byol := PriceRate{
			RateID:     "offer-1#APN1-InstanceUsage:db.m8i.2xl#Oracle",
			Label:      "db.m8i.2xlarge / Oracle",
			Attributes: map[string]string{"instance_type": "db.m8i.2xlarge", "engine": "Oracle"},
			PriceUSD:   0.832,
			Operation:  "CreateDBInstance:0005",
		}
		opLicense := map[string]string{
			"CreateDBInstance:0020": "License included",
			"CreateDBInstance:0005": "Bring your own license",
		}

		got := applySavingsPlanLicenseModel([]PriceRate{licenseIncluded, byol}, opLicense)

		if got[0].RateID == got[1].RateID {
			t.Fatalf("RateID collision: both got %q, want distinct RateIDs", got[0].RateID)
		}
		if got[0].Label == got[1].Label {
			t.Fatalf("Label collision: both got %q, want distinct Labels", got[0].Label)
		}
	})
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
	rates, _, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "ec2", resourceServiceSpecs["ec2"])
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
	rates, _, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "ec2", resourceServiceSpecs["ec2"])
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
	rates, _, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "ec2", resourceServiceSpecs["ec2"])
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
	rates, _, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "ec2", resourceServiceSpecs["ec2"])
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
	if _, _, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "ec2", resourceServiceSpecs["ec2"]); err != nil {
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
	if _, _, err := fetchOnDemandAndReserved(context.Background(), client, "ap-northeast-1", "rds", resourceServiceSpecs["rds"]); err != nil {
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
			// 2 ページ目は instanceType を変え、意図的に別レートにする。両ページが同一内容
			// だと issue 0052 対応の dedupeSavingsPlanRates が (正しく) 1 件にまとめてしまい、
			// 「ページネーションで両ページ分のデータが結果に含まれること」を検証できなくなる。
			instanceType := "m5.large"
			if in.NextToken != nil {
				instanceType = "m5.xlarge"
			}
			rate := newSPRate("APN1-BoxUsage:"+instanceType, "0.08", &sptypes.ParentSavingsPlanOffering{
				OfferingId: strPtr("o1"), PaymentOption: sptypes.SavingsPlanPaymentOptionNoUpfront,
				PlanType: sptypes.SavingsPlanTypeCompute, DurationSeconds: 31536000,
			}, map[string]string{"instanceType": instanceType, "productDescription": "Linux"})
			rate.ServiceCode = sptypes.SavingsPlanRateServiceCodeEc2
			if in.NextToken == nil {
				token := "page2"
				return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{SearchResults: []sptypes.SavingsPlanOfferingRate{rate}, NextToken: &token}, nil
			}
			return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{SearchResults: []sptypes.SavingsPlanOfferingRate{rate}}, nil
		},
	}
	rates, err := fetchSavingsPlans(context.Background(), client, "ap-northeast-1", savingsPlanServiceSpecs["compute-sp"])
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
			rate.ServiceCode = sptypes.SavingsPlanRateServiceCodeRds
			emptyToken := ""
			return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
				SearchResults: []sptypes.SavingsPlanOfferingRate{rate},
				NextToken:     &emptyToken,
			}, nil
		},
	}
	rates, err := fetchSavingsPlans(context.Background(), client, "ap-northeast-1", savingsPlanServiceSpecs["database-sp"])
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
	if _, err := fetchSavingsPlans(context.Background(), client, "ap-northeast-1", savingsPlanServiceSpecs["ec2-instance-sp"]); err == nil {
		t.Fatal("fetchSavingsPlans() err = nil, want error")
	}
}

func TestResourceKindFromServiceCode(t *testing.T) {
	tests := []struct {
		name        string
		serviceCode sptypes.SavingsPlanRateServiceCode
		wantKind    string
		wantOK      bool
	}{
		{name: "ec2", serviceCode: sptypes.SavingsPlanRateServiceCodeEc2, wantKind: "ec2", wantOK: true},
		{name: "fargate maps to ecs kind", serviceCode: sptypes.SavingsPlanRateServiceCodeFargate, wantKind: "ecs", wantOK: true},
		{name: "rds", serviceCode: sptypes.SavingsPlanRateServiceCodeRds, wantKind: "rds", wantOK: true},
		{name: "elasticache", serviceCode: sptypes.SavingsPlanRateServiceCodeElasticache, wantKind: "elasticache", wantOK: true},
		{name: "unrecognized (e.g. Lambda, out of v1 scope)", serviceCode: sptypes.SavingsPlanRateServiceCodeLambda, wantKind: "", wantOK: false},
		{name: "empty", serviceCode: "", wantKind: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, ok := resourceKindFromServiceCode(tt.serviceCode)
			if kind != tt.wantKind || ok != tt.wantOK {
				t.Errorf("resourceKindFromServiceCode(%q) = (%q, %v), want (%q, %v)", tt.serviceCode, kind, ok, tt.wantKind, tt.wantOK)
			}
		})
	}
}

// TestFetchSavingsPlansDispatchesMixedServiceCodes は issue 0055 の compute-sp
// (EC2 と Fargate の行が混在する) / database-sp (RDS と ElastiCache の行が混在する)
// を想定し、行ごとの ServiceCode によって正しい正規化関数へ振り分けられることを検証する。
// スラッグ単位で振り分けていた旧実装なら、compute-sp の Fargate 行が instanceSavingsPlanRate
// (EC2/RDS/ElastiCache 用) に誤って流れ込み、database-sp の ElastiCache Serverless
// 除外が効かなくなる (どちらも本行の ServiceCode を見ることで正しく振り分けられる)。
func TestFetchSavingsPlansDispatchesMixedServiceCodes(t *testing.T) {
	offering := &sptypes.ParentSavingsPlanOffering{
		OfferingId: strPtr("o1"), PaymentOption: sptypes.SavingsPlanPaymentOptionNoUpfront,
		PlanType: sptypes.SavingsPlanTypeCompute, DurationSeconds: 31536000,
	}

	t.Run("compute-sp: EC2 row and Fargate row are each parsed by their own logic", func(t *testing.T) {
		ec2Rate := newSPRate("APN1-BoxUsage:m5.large", "0.08", offering, map[string]string{
			"instanceType": "m5.large", "productDescription": "Linux",
		})
		ec2Rate.ServiceCode = sptypes.SavingsPlanRateServiceCodeEc2
		fargateRate := newSPRate("APN1-Fargate-vCPU-Hours:perCPU", "0.03", offering, map[string]string{
			"tenancy": "shared", "region": "ap-northeast-1",
		})
		fargateRate.ServiceCode = sptypes.SavingsPlanRateServiceCodeFargate

		client := &fakeSavingsPlansClient{
			describe: func(*savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
				return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
					SearchResults: []sptypes.SavingsPlanOfferingRate{ec2Rate, fargateRate},
				}, nil
			},
		}
		got, err := fetchSavingsPlans(context.Background(), client, "ap-northeast-1", savingsPlanServiceSpecs["compute-sp"])
		if err != nil {
			t.Fatalf("fetchSavingsPlans() err = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2 (EC2 row misparsed as Fargate or vice versa, or one was dropped)", len(got))
		}
		var ec2Row, fargateRow *PriceRate
		for i := range got {
			switch got[i].Attributes["instance_type"] {
			case "m5.large":
				ec2Row = &got[i]
			default:
				fargateRow = &got[i]
			}
		}
		if ec2Row == nil || ec2Row.Attributes["os"] != "Linux" {
			t.Errorf("EC2 row not parsed via instanceSavingsPlanRate: %+v", ec2Row)
		}
		if fargateRow == nil || fargateRow.Label != "Fargate vCPU / Linux / x86" {
			t.Errorf("Fargate row not parsed via ecsSavingsPlanRate: %+v", fargateRow)
		}
	})

	t.Run("database-sp: ElastiCache Serverless row is still excluded when dispatched by ServiceCode", func(t *testing.T) {
		serverlessRate := newSPRate("APN1-ElastiCacheProcessingUnits:Valkey", "0.0000000019", offering, map[string]string{
			"productDescription": "Valkey",
		})
		serverlessRate.ServiceCode = sptypes.SavingsPlanRateServiceCodeElasticache
		nodeRate := newSPRate("APN1-NodeUsage:cache.m7g.2xlarge", "0.51", offering, map[string]string{
			"instanceType": "cache.m7g.2xlarge", "productDescription": "Valkey",
		})
		nodeRate.ServiceCode = sptypes.SavingsPlanRateServiceCodeElasticache

		client := &fakeSavingsPlansClient{
			describe: func(*savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
				return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
					SearchResults: []sptypes.SavingsPlanOfferingRate{serverlessRate, nodeRate},
				}, nil
			},
		}
		got, err := fetchSavingsPlans(context.Background(), client, "ap-northeast-1", savingsPlanServiceSpecs["database-sp"])
		if err != nil {
			t.Fatalf("fetchSavingsPlans() err = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("len(got) = %d, want 1 (ElastiCache Serverless processing-unit row must be excluded)", len(got))
		}
		if got[0].Attributes["instance_type"] != "cache.m7g.2xlarge" {
			t.Errorf("unexpected surviving row: %+v", got[0])
		}
	})

	t.Run("unrecognized ServiceCode row is dropped, not fatal", func(t *testing.T) {
		validRate := newSPRate("APN1-BoxUsage:m5.large", "0.08", offering, map[string]string{
			"instanceType": "m5.large", "productDescription": "Linux",
		})
		validRate.ServiceCode = sptypes.SavingsPlanRateServiceCodeEc2
		unknownRate := newSPRate("APN1-Unknown", "0.01", offering, map[string]string{})
		unknownRate.ServiceCode = sptypes.SavingsPlanRateServiceCodeLambda

		client := &fakeSavingsPlansClient{
			describe: func(*savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
				return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
					SearchResults: []sptypes.SavingsPlanOfferingRate{validRate, unknownRate},
				}, nil
			},
		}
		got, err := fetchSavingsPlans(context.Background(), client, "ap-northeast-1", savingsPlanServiceSpecs["compute-sp"])
		if err != nil {
			t.Fatalf("fetchSavingsPlans() err = %v, want nil (an unrecognized row must be skipped, not fatal)", err)
		}
		if len(got) != 1 {
			t.Fatalf("len(got) = %d, want 1 (unrecognized ServiceCode row must be dropped)", len(got))
		}
	})
}

// TestGetPricingOrchestration は issue 0055 でリソースサービスと Savings Plans
// サービスに分離されたオーケストレーションを検証する。分離前は 1 つの getPricing が
// On-Demand/RI (必須) と SP (best-effort、失敗時は partial に縮退) を同時に扱っていたが、
// 分離後はリソースサービス (getResourcePricing: On-Demand/RI のみ、失敗は即エラー) と
// SP サービス (getSavingsPlanPricing: SP 自体が必須データ、ライセンス逆引きの補助
// On-Demand 取得のみ best-effort) に分かれる。
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
				PlanType: sptypes.SavingsPlanTypeEc2Instance, DurationSeconds: 31536000,
			}, map[string]string{"instanceType": "m5.large", "productDescription": "Linux"})
			rate.ServiceCode = sptypes.SavingsPlanRateServiceCodeEc2
			return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{SearchResults: []sptypes.SavingsPlanOfferingRate{rate}}, nil
		},
	}
	failSP := &fakeSavingsPlansClient{
		describe: func(*savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			return nil, errors.New("sp unavailable")
		},
	}

	t.Run("resource service fetches on-demand/RI only (no SP mixed in)", func(t *testing.T) {
		table, err := getResourcePricing(context.Background(), okPricing, "ap-northeast-1", "ec2", resourceServiceSpecs["ec2"])
		if err != nil {
			t.Fatalf("getResourcePricing() err = %v", err)
		}
		if table.LicenseUnresolved {
			t.Error("LicenseUnresolved = true, want false (resource services never set this field)")
		}
		for _, r := range table.Rates {
			if r.Model == "savings_plan" {
				t.Error("resource service table contains a savings_plan rate; resource services must not fetch SP after issue 0055")
			}
		}
	})

	t.Run("resource service on-demand failure aborts entirely", func(t *testing.T) {
		_, err := getResourcePricing(context.Background(), failPricing, "ap-northeast-1", "ec2", resourceServiceSpecs["ec2"])
		if err == nil {
			t.Fatal("getResourcePricing() err = nil, want error")
		}
	})

	t.Run("savings plan service resolves license via auxiliary on-demand fetch", func(t *testing.T) {
		table, err := getSavingsPlanPricing(context.Background(), okPricing, okSP, "ap-northeast-1", "ec2-instance-sp", savingsPlanServiceSpecs["ec2-instance-sp"])
		if err != nil {
			t.Fatalf("getSavingsPlanPricing() err = %v", err)
		}
		if table.LicenseUnresolved {
			t.Error("LicenseUnresolved = true, want false (auxiliary on-demand fetch succeeded)")
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

	t.Run("auxiliary license fetch failure degrades to LicenseUnresolved without failing the request", func(t *testing.T) {
		table, err := getSavingsPlanPricing(context.Background(), failPricing, okSP, "ap-northeast-1", "ec2-instance-sp", savingsPlanServiceSpecs["ec2-instance-sp"])
		if err != nil {
			t.Fatalf("getSavingsPlanPricing() err = %v, want nil (auxiliary license fetch failure must degrade, not error)", err)
		}
		if !table.LicenseUnresolved {
			t.Error("LicenseUnresolved = false, want true")
		}
		hasSP := false
		for _, r := range table.Rates {
			if r.Model == "savings_plan" {
				hasSP = true
			}
		}
		if !hasSP {
			t.Error("SP rates must still be present despite the auxiliary license fetch failing (SP is the primary data source, not the aux fetch)")
		}
	})

	t.Run("savings plan fetch failure aborts entirely (SP is the primary data source)", func(t *testing.T) {
		_, err := getSavingsPlanPricing(context.Background(), okPricing, failSP, "ap-northeast-1", "ec2-instance-sp", savingsPlanServiceSpecs["ec2-instance-sp"])
		if err == nil {
			t.Fatal("getSavingsPlanPricing() err = nil, want error (unlike the pre-0055 model, SP failure must not degrade here)")
		}
	})
}

// 実際に AWS Price List API から取得した生 JSON (issue 0053 実装時にライブ検証済み: RDS
// Oracle の db.m8i.2xlarge は License Included/BYOL の 2 SKU を持ち、operation は
// それぞれ "CreateDBInstance:0020"/"CreateDBInstance:0005") をもとにしたテスト用ドキュメント。
const rdsOracleLicenseIncludedDoc = `{
  "product": {"sku": "SKU20", "productFamily": "Database Instance", "attributes": {
    "instanceType": "db.m8i.2xlarge", "databaseEngine": "Oracle", "deploymentOption": "Single-AZ",
    "storage": "EBS Only", "licenseModel": "License included", "operation": "CreateDBInstance:0020",
    "regionCode": "ap-northeast-1", "usagetype": "APN1-InstanceUsage:db.m8i.2xl"
  }},
  "terms": {
    "OnDemand": {"SKU20.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
      "SKU20.OTC1.RC1": {"rateCode": "SKU20.OTC1.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "3.2900000000"}}
    }}}
  }
}`

const rdsOracleBYOLDoc = `{
  "product": {"sku": "SKU21", "productFamily": "Database Instance", "attributes": {
    "instanceType": "db.m8i.2xlarge", "databaseEngine": "Oracle", "deploymentOption": "Single-AZ",
    "storage": "EBS Only", "licenseModel": "Bring your own license", "operation": "CreateDBInstance:0005",
    "regionCode": "ap-northeast-1", "usagetype": "APN1-InstanceUsage:db.m8i.2xl"
  }},
  "terms": {
    "OnDemand": {"SKU21.OTC1": {"offerTermCode": "OTC1", "termAttributes": {}, "priceDimensions": {
      "SKU21.OTC1.RC1": {"rateCode": "SKU21.OTC1.RC1", "unit": "Hrs", "pricePerUnit": {"USD": "1.6600000000"}}
    }}}
  }
}`

// TestGetPricingResolvesSavingsPlanLicenseModel は issue 0053 の再現シナリオを end-to-end で
// 検証する。RDS Oracle の License Included/BYOL は Savings Plans 側では
// offeringId+usageType+productDescription が完全一致する (Operation コードのみが異なる)
// ため、On-Demand 側からのライセンスモデル逆引きが無いと price_usd だけが異なる同一 RateID の
// 行が残ってしまう (dedupeSavingsPlanRates は price_usd が異なるため統合しない)。
func TestGetPricingResolvesSavingsPlanLicenseModel(t *testing.T) {
	pricingClient := &fakePricingClient{
		getProducts: func(*pricing.GetProductsInput) (*pricing.GetProductsOutput, error) {
			return &pricing.GetProductsOutput{PriceList: []string{rdsOracleLicenseIncludedDoc, rdsOracleBYOLDoc}}, nil
		},
	}

	offeringID := "8870e805-fb04-4245-820e-231a54e5121b"
	offering := &sptypes.ParentSavingsPlanOffering{
		OfferingId: &offeringID, PaymentOption: sptypes.SavingsPlanPaymentOptionNoUpfront,
		PlanType: sptypes.SavingsPlanTypeDatabase, DurationSeconds: 31536000,
	}
	licenseIncludedOp := "CreateDBInstance:0020"
	byolOp := "CreateDBInstance:0005"
	licenseIncludedRate := newSPRate("APN1-InstanceUsage:db.m8i.2xl", "1.6448", offering, map[string]string{
		"instanceType": "db.m8i.2xlarge", "productDescription": "Oracle",
	})
	licenseIncludedRate.Operation = &licenseIncludedOp
	licenseIncludedRate.ServiceCode = sptypes.SavingsPlanRateServiceCodeRds
	byolRate := newSPRate("APN1-InstanceUsage:db.m8i.2xl", "0.832", offering, map[string]string{
		"instanceType": "db.m8i.2xlarge", "productDescription": "Oracle",
	})
	byolRate.Operation = &byolOp
	byolRate.ServiceCode = sptypes.SavingsPlanRateServiceCodeRds

	spClient := &fakeSavingsPlansClient{
		describe: func(*savingsplans.DescribeSavingsPlansOfferingRatesInput) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
				SearchResults: []sptypes.SavingsPlanOfferingRate{licenseIncludedRate, byolRate},
			}, nil
		},
	}

	table, err := getSavingsPlanPricing(context.Background(), pricingClient, spClient, "ap-northeast-1", "database-sp", savingsPlanServiceSpecs["database-sp"])
	if err != nil {
		t.Fatalf("getSavingsPlanPricing() err = %v", err)
	}
	if table.LicenseUnresolved {
		t.Error("LicenseUnresolved = true, want false (auxiliary on-demand fetch succeeded)")
	}

	var spRates []PriceRate
	for _, r := range table.Rates {
		if r.Model == "savings_plan" {
			spRates = append(spRates, r)
		}
	}
	if len(spRates) != 2 {
		t.Fatalf("savings_plan rates = %d, want 2 (must not collapse into 1: prices genuinely differ)", len(spRates))
	}
	if spRates[0].RateID == spRates[1].RateID {
		t.Errorf("RateID collision: both got %q, want distinct RateIDs (issue 0053)", spRates[0].RateID)
	}
	if spRates[0].Label == spRates[1].Label {
		t.Errorf("Label collision: both got %q, want distinct Labels (issue 0053)", spRates[0].Label)
	}
	for _, r := range spRates {
		if r.Attributes["license_model"] == "" {
			t.Errorf("rate %+v missing license_model attribute", r)
		}
	}
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
		{service: "compute-sp", wantErr: false},
		{service: "ec2-instance-sp", wantErr: false},
		{service: "database-sp", wantErr: false},
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
