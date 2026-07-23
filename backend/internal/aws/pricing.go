package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	sptypes "github.com/aws/aws-sdk-go-v2/service/savingsplans/types"
	"golang.org/x/sync/singleflight"
)

// PriceTable is a normalized price rate table for one service/region. Since
// issue 0055, each service is either a resource service (On-Demand/Reserved
// Instance rates only) or a Savings Plans service (Savings Plans rates only);
// no table mixes both.
type PriceTable struct {
	Service   string    `json:"service"`
	Region    string    `json:"region"`
	FetchedAt time.Time `json:"fetched_at"`
	// LicenseUnresolved is set by Savings Plans services only, when the
	// best-effort auxiliary On-Demand fetch used to resolve licenseModel
	// (issue 0053) fails. The Savings Plans rates themselves are still
	// present and complete; only the license_model distinction among them is
	// unavailable. This is a distinct concept from a failed primary fetch
	// (which aborts the request entirely, see getSavingsPlanPricing) and is
	// deliberately not represented via a generic partial/missing-models pair,
	// to avoid colliding with resource services (which never degrade this
	// way after the SP/resource split).
	LicenseUnresolved bool        `json:"license_unresolved"`
	Rates             []PriceRate `json:"rates"`
}

// PriceRate is one selectable rate row (On-Demand, Reserved Instance, or
// Savings Plan). Attributes holds curated keys used for filtering, not
// display; Label is the display string.
type PriceRate struct {
	RateID     string            `json:"rate_id"`
	Model      string            `json:"model"`
	Group      string            `json:"group"`
	Label      string            `json:"label"`
	Attributes map[string]string `json:"attributes"`
	Term       PriceTerm         `json:"term"`
	Unit       string            `json:"unit"`
	PriceUSD   float64           `json:"price_usd"`
	UpfrontUSD float64           `json:"upfront_usd"`
	Currency   string            `json:"currency"`

	// Operation は savings_plan レートに対してのみ savingsPlanRateFrom が設定する内部専用
	// フィールド (json:"-" のため API レスポンスには出ない)。SavingsPlanOfferingRate.Operation
	// をそのまま保持し、applySavingsPlanLicenseModel が On-Demand/Reserved 側から集めた
	// operation→licenseModel 対応表 (issue 0053) を逆引きするキーとして使う。on_demand/
	// reserved のレートおよび ECS の savings_plan レートでは常に空文字列のまま。
	Operation string `json:"-"`
}

// PriceTerm describes Reserved Instance / Savings Plan purchase conditions.
// All fields are nil for on_demand rates. OfferingClass is nil for services
// whose Reserved Instances have no offering-class distinction, and for all
// Savings Plans rates (Savings Plans don't have an offering class).
type PriceTerm struct {
	Lease         *string `json:"lease"`
	OfferingClass *string `json:"offering_class"`
	Payment       *string `json:"payment"`
}

// pricingAPI is the subset of the Price List Service client this package
// uses. Tests inject a hand-written fake.
type pricingAPI interface {
	GetProducts(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error)
}

// savingsPlansAPI is the subset of the Savings Plans client this package
// uses. Tests inject a hand-written fake.
type savingsPlansAPI interface {
	DescribeSavingsPlansOfferingRates(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error)
}

// ec2SpotAPI is the subset of the EC2 client this package uses for Spot
// pricing (issue 0056). Tests inject a hand-written fake.
type ec2SpotAPI interface {
	DescribeSpotPriceHistory(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error)
}

// resourceServiceSpec maps a thief resource pricing service slug (ec2/rds/
// elasticache/ecs) to the AWS Price List service code needed to fetch its
// On-Demand/Reserved Instance rates. Since issue 0055, resource services no
// longer fetch Savings Plans rates (see savingsPlanServiceSpec/
// savingsPlanServiceSpecs): Savings Plans are independent services because
// they apply flexibly across multiple resource services (e.g. Compute SP
// spans EC2 and Fargate), which made them appear duplicated across resource
// cards under the old combined model.
type resourceServiceSpec struct {
	awsServiceCode string
	// productFamily is the Price List "productFamily" attribute value that
	// identifies the instance/task-hour line items this service cares about,
	// used both as a GetProducts API-side filter and as a defense-in-depth
	// check in instanceOnDemandRatesFromDocument. Confirmed against live
	// data (see issue 0045 follow-up): AmazonRDS/AmazonElastiCache/AmazonEC2
	// each return many unrelated productFamily values (e.g. "CPU Credits",
	// "Storage", "Dedicated Host", "ElastiCache Serverless") under the same
	// ServiceCode, and without this filter those rows pollute the normalized
	// table with empty/meaningless labels and non-Hrs units.
	productFamily string
	riSupported   bool
}

// resourceServiceSpecs is the fixed allowlist of supported resource pricing
// services, matching the RI support matrix confirmed against live AWS data
// (see issue 0045): EC2/RDS/ElastiCache have RI; ECS (Fargate) has no RI.
var resourceServiceSpecs = map[string]resourceServiceSpec{
	"ec2": {
		awsServiceCode: "AmazonEC2",
		productFamily:  "Compute Instance",
		riSupported:    true,
	},
	"rds": {
		awsServiceCode: "AmazonRDS",
		productFamily:  "Database Instance",
		riSupported:    true,
	},
	"elasticache": {
		awsServiceCode: "AmazonElastiCache",
		productFamily:  "Cache Instance",
		riSupported:    true,
	},
	"ecs": {
		awsServiceCode: "AmazonECS",
		productFamily:  "Compute",
		riSupported:    false,
	},
}

// savingsPlanServiceSpec maps a thief Savings Plans service slug
// (compute-sp/ec2-instance-sp/database-sp) to the DescribeSavingsPlansOfferingRates
// filter values and the resource service whose On-Demand data resolves
// licenseModel (issue 0053) for this SP type.
type savingsPlanServiceSpec struct {
	planTypes    []sptypes.SavingsPlanType
	serviceCodes []sptypes.SavingsPlanRateServiceCode
	// licenseSource is a resourceServiceSpecs key (or "" for none) whose
	// On-Demand data is fetched best-effort to build the operation→
	// licenseModel lookup table (recordOperationLicenseModel) used to
	// resolve licenseModel on this SP's rates (applySavingsPlanLicenseModel).
	// ElastiCache and Fargate have no licenseModel concept
	// (recordOperationLicenseModel only records entries with both operation
	// and licenseModel non-empty), so RDS alone is sufficient for
	// database-sp and EC2 alone is sufficient for compute-sp/ec2-instance-sp;
	// no cross-serviceCode merge of lookup tables is needed.
	licenseSource string
}

// savingsPlanServiceSpecs is the fixed allowlist of supported Savings Plans
// services. Lambda/SageMaker/etc. are valid Compute SP serviceCodes too, but
// are out of scope for v1 (see issue 0055 design notes).
var savingsPlanServiceSpecs = map[string]savingsPlanServiceSpec{
	"compute-sp": {
		planTypes:     []sptypes.SavingsPlanType{sptypes.SavingsPlanTypeCompute},
		serviceCodes:  []sptypes.SavingsPlanRateServiceCode{sptypes.SavingsPlanRateServiceCodeEc2, sptypes.SavingsPlanRateServiceCodeFargate},
		licenseSource: "ec2",
	},
	"ec2-instance-sp": {
		planTypes:     []sptypes.SavingsPlanType{sptypes.SavingsPlanTypeEc2Instance},
		serviceCodes:  []sptypes.SavingsPlanRateServiceCode{sptypes.SavingsPlanRateServiceCodeEc2},
		licenseSource: "ec2",
	},
	"database-sp": {
		planTypes:     []sptypes.SavingsPlanType{sptypes.SavingsPlanTypeDatabase},
		serviceCodes:  []sptypes.SavingsPlanRateServiceCode{sptypes.SavingsPlanRateServiceCodeRds, sptypes.SavingsPlanRateServiceCodeElasticache},
		licenseSource: "rds",
	},
}

// EC2SpotService is the pricing service slug for EC2 Spot rates (issue
// 0056). Unlike resourceServiceSpecs/savingsPlanServiceSpecs members,
// ec2-spot has no catalog spec: it is a live, dynamically-priced feed
// (ec2:DescribeSpotPriceHistory) with no stable per-service/region catalog
// to cache on disk (see getEC2SpotPricing and handlers_pricing.go, which
// must route ec2-spot around pricecache.Load/Save entirely — only the
// singleflight in pricecache.Fetch applies).
const EC2SpotService = "ec2-spot"

// ValidatePricingService returns ErrInvalidPricingService unless service is
// one of the supported pricing service slugs (the union of
// resourceServiceSpecs, savingsPlanServiceSpecs, and EC2SpotService).
func ValidatePricingService(service string) error {
	if service == EC2SpotService {
		return nil
	}
	if _, ok := resourceServiceSpecs[service]; ok {
		return nil
	}
	if _, ok := savingsPlanServiceSpecs[service]; ok {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrInvalidPricingService, service)
}

// GetPricing returns the normalized price table for service/region, using
// profile's credentials to call the Price List, Savings Plans, and/or EC2
// APIs (only the client(s) the service actually needs are created). Price
// List/Savings Plans endpoints are global services; both clients are pinned
// to us-east-1 (mirroring newCostExplorerClient), and region is used only as
// an API-side filter value, not a client region. EC2 Spot is the exception:
// DescribeSpotPriceHistory is a regional endpoint, so its client is created
// for region itself (see newEC2Client, already used by ec2.go).
func GetPricing(ctx context.Context, profile, region, service string) (*PriceTable, error) {
	if service == EC2SpotService {
		client, err := newEC2Client(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		return getEC2SpotPricing(ctx, client, region)
	}
	if spec, ok := resourceServiceSpecs[service]; ok {
		pricingClient, err := newPricingClient(ctx, profile)
		if err != nil {
			return nil, err
		}
		return getResourcePricing(ctx, pricingClient, region, service, spec)
	}
	if spec, ok := savingsPlanServiceSpecs[service]; ok {
		spClient, err := newSavingsPlansClient(ctx, profile)
		if err != nil {
			return nil, err
		}
		var pricingClient pricingAPI
		if spec.licenseSource != "" {
			pricingClient, err = newPricingClient(ctx, profile)
			if err != nil {
				return nil, err
			}
		}
		return getSavingsPlanPricing(ctx, pricingClient, spClient, region, service, spec)
	}
	return nil, fmt.Errorf("%w: %q", ErrInvalidPricingService, service)
}

// getResourcePricing fetches On-Demand/Reserved Instance rates only (see
// resourceServiceSpec doc). Unlike the pre-0055 combined fetch, there is no
// partial-degradation path here: a resource service's only data source is
// this one fetch, so its failure always aborts the request.
func getResourcePricing(ctx context.Context, pc pricingAPI, region, service string, spec resourceServiceSpec) (*PriceTable, error) {
	rates, _, err := fetchOnDemandAndReserved(ctx, pc, region, service, spec)
	if err != nil {
		return nil, fmt.Errorf("fetch on-demand/reserved pricing for %s: %w", service, err)
	}
	sortPriceRates(rates)
	return &PriceTable{
		Service: service,
		Region:  region,
		Rates:   rates,
	}, nil
}

// getSavingsPlanPricing fetches Savings Plans rates (the primary, required
// data source for this service — its failure aborts the request, unlike the
// best-effort auxiliary license lookup below) and, if spec.licenseSource is
// set, best-effort resolves licenseModel via an auxiliary On-Demand fetch
// scoped to that resource service. Failure of the auxiliary fetch degrades to
// LicenseUnresolved=true rather than failing the whole request or reusing the
// old savings_plan-missing partial representation (see PriceTable doc).
func getSavingsPlanPricing(ctx context.Context, pc pricingAPI, sp savingsPlansAPI, region, service string, spec savingsPlanServiceSpec) (*PriceTable, error) {
	spRates, err := fetchSavingsPlans(ctx, sp, region, spec)
	if err != nil {
		return nil, fmt.Errorf("fetch savings plans pricing for %s: %w", service, err)
	}

	table := &PriceTable{
		Service: service,
		Region:  region,
		Rates:   spRates,
	}
	if spec.licenseSource == "" {
		sortPriceRates(table.Rates)
		return table, nil
	}

	opLicense, licErr := fetchAuxLicenseModel(ctx, pc, region, spec.licenseSource)
	if licErr != nil {
		slog.Warn("fetch auxiliary on-demand pricing for savings plan license model resolution failed; degrading to unresolved license",
			"service", service, "region", region, "license_source", spec.licenseSource, "err", licErr)
		table.LicenseUnresolved = true
		sortPriceRates(table.Rates)
		return table, nil
	}
	table.Rates = applySavingsPlanLicenseModel(table.Rates, opLicense)
	sortPriceRates(table.Rates)
	return table, nil
}

// licenseAuxGroup dedupes concurrent auxiliary On-Demand fetches (keyed by
// awsServiceCode+region) issued purely to resolve Savings Plans licenseModel.
// compute-sp and ec2-instance-sp both use licenseSource="ec2", so concurrent
// requests for the same region (the frontend fetches all pricing services in
// parallel) would otherwise call GetProducts for AmazonEC2 twice; under
// throttling, that doubles the chance the license lookup degrades. This is a
// robustness measure (see PriceTable.LicenseUnresolved doc), not a
// performance optimization: whether to also coalesce with the ec2 resource
// service's own primary fetch is left to issue 0058's measurement.
var licenseAuxGroup singleflight.Group

func fetchAuxLicenseModel(ctx context.Context, pc pricingAPI, region, licenseSource string) (map[string]string, error) {
	spec := resourceServiceSpecs[licenseSource]
	key := spec.awsServiceCode + "|" + region
	v, err, _ := licenseAuxGroup.Do(key, func() (any, error) {
		_, opLicense, ferr := fetchOnDemandAndReserved(ctx, pc, region, licenseSource, spec)
		if ferr != nil {
			return nil, ferr
		}
		return opLicense, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(map[string]string), nil
}

func newPricingClient(ctx context.Context, profile string) (*pricing.Client, error) {
	return NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *pricing.Client {
		return pricing.NewFromConfig(cfg)
	})
}

func newSavingsPlansClient(ctx context.Context, profile string) (*savingsplans.Client, error) {
	return NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *savingsplans.Client {
		return savingsplans.NewFromConfig(cfg)
	})
}

func sortPriceRates(rates []PriceRate) {
	sort.Slice(rates, func(i, j int) bool {
		if rates[i].Group != rates[j].Group {
			return rates[i].Group < rates[j].Group
		}
		if rates[i].Label != rates[j].Label {
			return rates[i].Label < rates[j].Label
		}
		return rates[i].RateID < rates[j].RateID
	})
}

// ---- On-Demand / Reserved Instance (pricing:GetProducts) ----

// priceListDocument is the price document schema for one PriceList[] entry.
// terms.OnDemand/terms.Reserved are flat maps keyed by "<sku>.<offerTermCode>"
// and priceDimensions is a flat map keyed by "<sku>.<offerTermCode>.<rateCode>";
// this package ignores those composite keys and reads the self-describing
// fields on each value instead, so it never depends on the key format.
type priceListDocument struct {
	Product struct {
		SKU           string            `json:"sku"`
		ProductFamily string            `json:"productFamily"`
		Attributes    map[string]string `json:"attributes"`
	} `json:"product"`
	Terms struct {
		OnDemand map[string]priceTermDoc `json:"OnDemand"`
		Reserved map[string]priceTermDoc `json:"Reserved"`
	} `json:"terms"`
}

type priceTermDoc struct {
	OfferTermCode   string                       `json:"offerTermCode"`
	TermAttributes  priceTermAttributesDoc       `json:"termAttributes"`
	PriceDimensions map[string]priceDimensionDoc `json:"priceDimensions"`
}

type priceTermAttributesDoc struct {
	LeaseContractLength string `json:"LeaseContractLength"`
	OfferingClass       string `json:"OfferingClass"`
	PurchaseOption      string `json:"PurchaseOption"`
}

type priceDimensionDoc struct {
	RateCode     string `json:"rateCode"`
	Unit         string `json:"unit"`
	PricePerUnit struct {
		USD string `json:"USD"`
	} `json:"pricePerUnit"`
}

func parsePriceDocument(raw string) (*priceListDocument, error) {
	var doc priceListDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("unmarshal price list document: %w", err)
	}
	if doc.Product.SKU == "" {
		return nil, fmt.Errorf("price list document missing product.sku")
	}
	return &doc, nil
}

func fetchOnDemandAndReserved(ctx context.Context, client pricingAPI, region, service string, spec resourceServiceSpec) ([]PriceRate, map[string]string, error) {
	filters := []pricingtypes.Filter{
		{Field: aws.String("regionCode"), Type: pricingtypes.FilterTypeTermMatch, Value: aws.String(region)},
		{Field: aws.String("productFamily"), Type: pricingtypes.FilterTypeTermMatch, Value: aws.String(spec.productFamily)},
	}
	if service == "ec2" {
		// 常用ケース (共有テナンシー・バンドルソフトウェアなし・実利用中の容量) に
		// 絞り込む。専有ホスト/専有インスタンス/未使用容量予約/SQL Server 等バンドル
		// SKU は v1 スコープ外 (GB 課金項目の対象外化と同じ「主要ケースに限定する」
		// 方針)。OS は実質的な比較軸として行に残す。
		filters = append(filters,
			pricingtypes.Filter{Field: aws.String("tenancy"), Type: pricingtypes.FilterTypeTermMatch, Value: aws.String("Shared")},
			pricingtypes.Filter{Field: aws.String("capacitystatus"), Type: pricingtypes.FilterTypeTermMatch, Value: aws.String("Used")},
			pricingtypes.Filter{Field: aws.String("preInstalledSw"), Type: pricingtypes.FilterTypeTermMatch, Value: aws.String("NA")},
		)
	}

	rates := []PriceRate{}
	opLicense := map[string]string{}
	var next *string
	for {
		out, err := client.GetProducts(ctx, &pricing.GetProductsInput{
			ServiceCode: aws.String(spec.awsServiceCode),
			Filters:     filters,
			NextToken:   next,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("get products: %w", err)
		}
		for _, raw := range out.PriceList {
			doc, perr := parsePriceDocument(raw)
			if perr != nil {
				slog.Warn("skip malformed price list document", "service", service, "err", perr)
				continue
			}
			rates = append(rates, priceRatesFromDocument(service, spec, *doc, opLicense)...)
		}
		// fetchSavingsPlans で確認した「最終ページで NextToken が nil ではなく空
		// 文字列になる」API の揺れに備え、こちらも空文字列を終端として扱う
		// (GetProducts では実データ上まだ観測していないが、同じ NextToken 型を
		// 使う AWS API 一般の防御として揃えておく)。
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		next = out.NextToken
	}
	return rates, opLicense, nil
}

func priceRatesFromDocument(service string, spec resourceServiceSpec, doc priceListDocument, opLicense map[string]string) []PriceRate {
	// GetProducts の productFamily フィルタ (fetchOnDemandAndReserved) が API 側
	// 不具合等で効かなかった場合の保険として、パース後にも二重チェックする。
	if doc.Product.ProductFamily != spec.productFamily {
		return nil
	}
	if service == "ecs" {
		return ecsOnDemandRatesFromDocument(doc)
	}
	return instanceOnDemandRatesFromDocument(service, spec, doc, opLicense)
}

func instanceOnDemandRatesFromDocument(service string, spec resourceServiceSpec, doc priceListDocument, opLicense map[string]string) []PriceRate {
	attrs := doc.Product.Attributes
	// Savings Plans の Properties にはライセンスモデル情報が無いため (issue 0053)、
	// On-Demand/Reserved の生属性から operation→licenseModel の対応を副産物として集める。
	recordOperationLicenseModel(opLicense, attrs)
	// EOL 延長サポート課金 (ExtendedSupport、RDS/ElastiCache の一部エンジンに存在) と
	// ElastiCache Valkey の同期耐久性オプション課金 (SyncDurability) は、いずれも同一
	// instanceType のノード基本料金 (NodeUsage) とは別メーターであり、同一 label で複数の
	// 紛らわしい重複行を生むため v1 スコープ外とする。
	if strings.Contains(attrs["usagetype"], "ExtendedSupport") ||
		strings.Contains(attrs["usagetype"], "SyncDurability") {
		return nil
	}
	label := instanceLabel(service, attrs)
	curated := curatedInstanceAttributes(service, attrs)

	var rates []PriceRate
	for _, term := range doc.Terms.OnDemand {
		for _, dim := range term.PriceDimensions {
			price, ok := parseUSD(dim.PricePerUnit.USD)
			if !ok {
				continue
			}
			rates = append(rates, PriceRate{
				RateID:     dim.RateCode,
				Model:      "on_demand",
				Group:      "On-Demand",
				Label:      label,
				Attributes: curated,
				Term:       PriceTerm{},
				Unit:       dim.Unit,
				PriceUSD:   price,
				UpfrontUSD: 0,
				Currency:   "USD",
			})
		}
	}

	if spec.riSupported {
		for _, term := range doc.Terms.Reserved {
			if rate, ok := reservedRateFromTerm(doc.Product.SKU, term, label, curated); ok {
				rates = append(rates, rate)
			}
		}
	}
	return rates
}

func reservedRateFromTerm(sku string, term priceTermDoc, label string, attrs map[string]string) (PriceRate, bool) {
	lease := term.TermAttributes.LeaseContractLength
	payment := term.TermAttributes.PurchaseOption
	if lease == "" || payment == "" {
		return PriceRate{}, false // 欠損: 不完全な RI 条件は破棄する
	}

	var hourly float64
	var hourlyFound bool
	var upfront float64
	for _, dim := range term.PriceDimensions {
		price, ok := parseUSD(dim.PricePerUnit.USD)
		if !ok {
			continue
		}
		if dim.Unit == "Quantity" {
			upfront = price
			continue
		}
		hourly = price
		hourlyFound = true
	}
	if !hourlyFound {
		return PriceRate{}, false
	}

	var offeringClass *string
	if term.TermAttributes.OfferingClass != "" {
		oc := strings.ToLower(term.TermAttributes.OfferingClass)
		offeringClass = &oc
	}

	return PriceRate{
		RateID:     sku + "." + term.OfferTermCode,
		Model:      "reserved",
		Group:      "Reserved Instance",
		Label:      label,
		Attributes: attrs,
		Term: PriceTerm{
			Lease:         strPtr(lease),
			OfferingClass: offeringClass,
			Payment:       strPtr(payment),
		},
		Unit:       "Hrs",
		PriceUSD:   hourly,
		UpfrontUSD: upfront,
		Currency:   "USD",
	}, true
}

func instanceLabel(service string, attrs map[string]string) string {
	switch service {
	case "ec2":
		return joinNonEmpty(" / ", attrs["instanceType"], attrs["operatingSystem"], attrs["tenancy"], attrs["licenseModel"])
	case "rds":
		storageLabel := "Standard"
		if rdsStorageType(attrs["storage"]) == "io_optimized" {
			storageLabel = "IO-Optimized"
		}
		return joinNonEmpty(" / ", attrs["instanceType"], attrs["databaseEngine"], attrs["deploymentOption"], storageLabel, attrs["licenseModel"])
	case "elasticache":
		return joinNonEmpty(" / ", attrs["instanceType"], attrs["cacheEngine"])
	default:
		return attrs["instanceType"]
	}
}

func curatedInstanceAttributes(service string, attrs map[string]string) map[string]string {
	out := map[string]string{}
	switch service {
	case "ec2":
		setIfNonEmpty(out, "instance_type", attrs["instanceType"])
		setIfNonEmpty(out, "instance_family", instanceFamily(attrs["instanceType"]))
		setIfNonEmpty(out, "os", attrs["operatingSystem"])
		setIfNonEmpty(out, "tenancy", attrs["tenancy"])
		setIfNonEmpty(out, "license_model", attrs["licenseModel"])
	case "rds":
		setIfNonEmpty(out, "instance_type", attrs["instanceType"])
		setIfNonEmpty(out, "instance_family", instanceFamily(attrs["instanceType"]))
		setIfNonEmpty(out, "engine", attrs["databaseEngine"])
		setIfNonEmpty(out, "deployment_option", attrs["deploymentOption"])
		setIfNonEmpty(out, "license_model", attrs["licenseModel"])
		out["storage_type"] = rdsStorageType(attrs["storage"])
	case "elasticache":
		setIfNonEmpty(out, "instance_type", attrs["instanceType"])
		setIfNonEmpty(out, "instance_family", instanceFamily(attrs["instanceType"]))
		setIfNonEmpty(out, "engine", attrs["cacheEngine"])
	}
	return out
}

// instanceFamily derives the instance family from an instance type by
// dropping the trailing size token (the segment after the last dot):
// "m5.large" -> "m5"; "db.r6g.4xlarge" -> "db.r6g"; "cache.t4g.micro" ->
// "cache.t4g". Instance types without a dot (malformed input, or an empty
// string) are returned unchanged so the caller's setIfNonEmpty skips them.
func instanceFamily(instanceType string) string {
	idx := strings.LastIndex(instanceType, ".")
	if idx < 0 {
		return instanceType
	}
	return instanceType[:idx]
}

// recordOperationLicenseModel は、1 件の Price List ドキュメントの生 attributes から
// operation コードと licenseModel の対応を dest に記録する (issue 0053)。Savings Plans の
// SavingsPlanOfferingRate.Operation から同じキーで引くための対応表を、On-Demand/Reserved を
// 解析するこの経路の副産物として組み立てる (追加の API 呼び出しを行わないため)。
// licenseModel という概念を持たないサービス/エンジン (ElastiCache、MySQL 等) の行は
// operation/licenseModel のいずれかが空文字列になり、その場合は何も記録しない。
// 同じ operation に異なる licenseModel が観測された場合は不整合として警告し、既存の値を
// 優先する (AWS の operation コードはインスタンスタイプを問わず一意な列挙値のはずであり、
// 実データでの不一致は想定外の入力として扱う)。
func recordOperationLicenseModel(dest map[string]string, attrs map[string]string) {
	op := attrs["operation"]
	lm := attrs["licenseModel"]
	if op == "" || lm == "" {
		return
	}
	if existing, ok := dest[op]; ok {
		if existing != lm {
			slog.Warn("operation code maps to multiple license models; keeping first-seen value",
				"operation", op, "kept", existing, "ignored", lm)
		}
		return
	}
	dest[op] = lm
}

// rdsStorageType normalizes the Price List "storage" attribute into a
// standard/io_optimized axis for filtering. IO-Optimized storage is an
// Aurora-only option (confirmed against live data: the "storage" attribute
// is the exact string "Aurora IO Optimization Mode" for IO-Optimized rows;
// every other RDS row, including Aurora's own default storage mode and all
// non-Aurora engines, reports "EBS Only" or an instance-store description).
func rdsStorageType(storage string) string {
	if storage == "Aurora IO Optimization Mode" {
		return "io_optimized"
	}
	return "standard"
}

// fargateUnitFromUsageType derives the normalized unit from a Fargate
// usagetype/usageType string. AWS's raw unit field is "hours" (pricing) or
// "Hrs" (savings plans) for every Fargate compute line item regardless of
// whether it bills vCPU or memory; the vCPU-vs-memory distinction only shows
// up as a usagetype substring. EphemeralStorage (a GB-Mo-style storage
// charge, out of v1 scope per the GB-billing exclusion) and the Windows OS
// license fee (neither vCPU nor memory) are excluded via ok=false.
func fargateUnitFromUsageType(usageType string) (unit string, ok bool) {
	if strings.Contains(usageType, "EphemeralStorage") {
		return "", false
	}
	switch {
	case strings.Contains(usageType, "vCPU-Hours"):
		return "vCPU-Hours", true
	case strings.Contains(usageType, "GB-Hours"):
		return "GB-Hours", true
	default:
		return "", false
	}
}

func fargateLabelAndAttributes(unit, os, arch string) (string, map[string]string) {
	kind := "Fargate vCPU"
	if unit == "GB-Hours" {
		kind = "Fargate Memory (GB)"
	}
	return fmt.Sprintf("%s / %s / %s", kind, os, arch), map[string]string{"os": os, "architecture": arch}
}

func ecsOnDemandRatesFromDocument(doc priceListDocument) []PriceRate {
	attrs := doc.Product.Attributes
	unit, ok := fargateUnitFromUsageType(attrs["usagetype"])
	if !ok {
		return nil
	}
	os := attrs["operatingSystem"]
	if os == "" {
		os = "Linux"
	}
	arch := "x86"
	if attrs["cpuArchitecture"] == "ARM" {
		arch = "ARM"
	}
	label, curated := fargateLabelAndAttributes(unit, os, arch)

	var rates []PriceRate
	for _, term := range doc.Terms.OnDemand {
		for _, dim := range term.PriceDimensions {
			price, ok := parseUSD(dim.PricePerUnit.USD)
			if !ok {
				continue
			}
			rates = append(rates, PriceRate{
				RateID:     dim.RateCode,
				Model:      "on_demand",
				Group:      "On-Demand",
				Label:      label,
				Attributes: curated,
				Term:       PriceTerm{},
				Unit:       unit,
				PriceUSD:   price,
				UpfrontUSD: 0,
				Currency:   "USD",
			})
		}
	}
	// ECS Fargate に Reserved Instances は存在しない (呼び出し元が
	// spec.riSupported=false で RI 解析自体をスキップする)。
	return rates
}

// ---- Savings Plans (savingsplans:DescribeSavingsPlansOfferingRates) ----

// resourceKindFromServiceCode maps a Savings Plans offering rate's
// ServiceCode to the resource-kind string (ec2/rds/elasticache/ecs) that
// savingsPlanRateFrom/instanceSavingsPlanRate/curatedInstanceAttributes use
// to select normalization logic (os vs. engine attribute, ElastiCache
// Serverless exclusion, Fargate parsing, etc.). This replaces the pre-0055
// dispatch on the thief service slug: compute-sp and database-sp each fetch
// rows for multiple resource kinds in one DescribeSavingsPlansOfferingRates
// call (e.g. compute-sp mixes AmazonEC2 and AmazonECS/Fargate rows), so the
// row's own ServiceCode — not the outer thief service — must decide how each
// row is parsed (dispatching by slug would misparse compute-sp's Fargate rows
// as EC2 instance rows, and would fail to exclude database-sp's ElastiCache
// Serverless rows).
func resourceKindFromServiceCode(sc sptypes.SavingsPlanRateServiceCode) (string, bool) {
	switch sc {
	case sptypes.SavingsPlanRateServiceCodeEc2:
		return "ec2", true
	case sptypes.SavingsPlanRateServiceCodeFargate:
		return "ecs", true
	case sptypes.SavingsPlanRateServiceCodeRds:
		return "rds", true
	case sptypes.SavingsPlanRateServiceCodeElasticache:
		return "elasticache", true
	default:
		return "", false
	}
}

func fetchSavingsPlans(ctx context.Context, client savingsPlansAPI, region string, spec savingsPlanServiceSpec) ([]PriceRate, error) {
	rates := []PriceRate{}
	var next *string
	for {
		out, err := client.DescribeSavingsPlansOfferingRates(ctx, &savingsplans.DescribeSavingsPlansOfferingRatesInput{
			SavingsPlanTypes: spec.planTypes,
			ServiceCodes:     spec.serviceCodes,
			Filters: []sptypes.SavingsPlanOfferingRateFilterElement{
				{Name: sptypes.SavingsPlanRateFilterAttributeRegion, Values: []string{region}},
			},
			NextToken: next,
		})
		if err != nil {
			return nil, fmt.Errorf("describe savings plans offering rates: %w", err)
		}
		for _, r := range out.SearchResults {
			kind, ok := resourceKindFromServiceCode(r.ServiceCode)
			if !ok {
				slog.Warn("skip savings plan offering rate with unrecognized service code",
					"service_code", string(r.ServiceCode))
				continue
			}
			if rate, ok := savingsPlanRateFrom(kind, r); ok {
				rates = append(rates, rate)
			}
		}
		// DescribeSavingsPlansOfferingRates は GetProducts と異なり、最終ページで
		// NextToken を nil ではなく空文字列で返すことを実データで確認した
		// (RDS/ElastiCache の Database SP は該当件数が少なく 1 ページで完結するため、
		// "" を次ページ扱いしてリクエストすると NextToken の正規表現バリデーションに
		// 落ちて 400 ValidationException になり、SP 取得全体が失敗していた)。
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		next = out.NextToken
	}
	return dedupeSavingsPlanRates(rates), nil
}

// dedupeSavingsPlanRates は、ユーザーに見える情報 (RateID を含む全フィールド) が完全一致する
// PriceRate を 1 件にまとめる。
//
// DescribeSavingsPlansOfferingRates は同一レートに対して複数の SearchResults エントリを返す
// ことがあり (issue 0052: RDS の Oracle/Db2 で実データ確認済み)、それらは内部的な Operation
// コード (例: "CreateDBInstance:0035" と "CreateDBInstance:0029") のみが異なり、Rate や
// Properties (productDescription/instanceType/region) を含む他の全フィールドは完全一致する。
// PriceRate.Operation は dedup キーに含めないため (dedupeSavingsPlanRateKey 参照)、本関数の
// 実行時点では license_model もまだ未解決 (getPricing が本関数の後に applySavingsPlanLicenseModel
// を適用する) なので、Operation 違いだけではまだユーザーから見て意味のある区別になっていない。
// RateID に Operation を追加して一意にする方式は採らない: 見た目には全く同じ内容の行が
// 依然として複数表示される問題 (React key の一意性は満たすが UX 上の重複は解消しない) が残る
// ため、表示・計算に使う値がすべて一致する行そのものを 1 件に統合する。統合対象は price_usd
// も含めて完全一致する行に限られるため、Operation 違いが本当に異なる price_usd を伴う場合
// (issue 0053: ライセンスモデル差) は別行として残り、後段の applySavingsPlanLicenseModel が
// license_model を反映して区別可能にする。
//
// dedup キーは可視フィールドを直接連結した文字列 (dedupeSavingsPlanRateKey) で、
// json.Marshal は使わない (issue 0058: testing.B での計測により、同等規模の入力で
// json.Marshal 版よりおよそ 2 倍高速かつメモリ確保が半減することを確認した上での変更)。
func dedupeSavingsPlanRates(rates []PriceRate) []PriceRate {
	seen := make(map[string]bool, len(rates))
	out := make([]PriceRate, 0, len(rates))
	var sb strings.Builder
	for _, r := range rates {
		key := dedupeSavingsPlanRateKey(&sb, r)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

// dedupeSavingsPlanRateKey builds the dedup key for one PriceRate into sb
// (reused across calls to avoid reallocating the builder's backing buffer per
// row) and returns a copy of the result (strings.Builder.String() copies the
// buffer, so the returned string stays valid after sb is reset for the next
// row). Attribute keys are sorted so two structurally-identical rows always
// produce the same key regardless of Go's randomized map iteration order.
func dedupeSavingsPlanRateKey(sb *strings.Builder, r PriceRate) string {
	sb.Reset()
	sb.WriteString(r.RateID)
	sb.WriteByte(0)
	sb.WriteString(r.Model)
	sb.WriteByte(0)
	sb.WriteString(r.Group)
	sb.WriteByte(0)
	sb.WriteString(r.Label)
	sb.WriteByte(0)
	attrKeys := make([]string, 0, len(r.Attributes))
	for k := range r.Attributes {
		attrKeys = append(attrKeys, k)
	}
	sort.Strings(attrKeys)
	for _, k := range attrKeys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(r.Attributes[k])
		sb.WriteByte(';')
	}
	sb.WriteByte(0)
	if r.Term.Lease != nil {
		sb.WriteString(*r.Term.Lease)
	}
	sb.WriteByte(0)
	if r.Term.OfferingClass != nil {
		sb.WriteString(*r.Term.OfferingClass)
	}
	sb.WriteByte(0)
	if r.Term.Payment != nil {
		sb.WriteString(*r.Term.Payment)
	}
	sb.WriteByte(0)
	sb.WriteString(r.Unit)
	sb.WriteByte(0)
	fmt.Fprintf(sb, "%v\x00%v\x00", r.PriceUSD, r.UpfrontUSD)
	sb.WriteString(r.Currency)
	return sb.String()
}

// applySavingsPlanLicenseModel は、同時に取得した On-Demand/Reserved データから作った
// operation→licenseModel 対応表 (recordOperationLicenseModel 参照) を使って、Savings Plans
// レートの RateID/Label/Attributes にライセンスモデルを反映する。
//
// DescribeSavingsPlansOfferingRates の Properties にはライセンスモデル情報が一切含まれない
// ため (issue 0053: RDS Oracle の BYOL/License Included、EC2 Windows の BYOL/標準ライセンス
// はいずれも offeringId+usageType+productDescription が完全一致するのに price_usd が本当に
// 異なる)、Operation コード経由で On-Demand 側のデータから逆引きする。Operation が対応表に
// 存在しない行 (オープンソースエンジンや Linux 等、ライセンスモデルという概念がそもそも無い
// 大多数の行) はそのまま変更しない。
func applySavingsPlanLicenseModel(rates []PriceRate, opLicense map[string]string) []PriceRate {
	for i := range rates {
		lm := opLicense[rates[i].Operation]
		if lm == "" {
			continue
		}
		rates[i].Label = joinNonEmpty(" / ", rates[i].Label, lm)
		rates[i].RateID += "#" + lm
		rates[i].Attributes["license_model"] = lm
	}
	return rates
}

func savingsPlanProperties(props []sptypes.SavingsPlanOfferingRateProperty) map[string]string {
	m := make(map[string]string, len(props))
	for _, p := range props {
		if p.Name == nil || p.Value == nil {
			continue
		}
		m[*p.Name] = *p.Value
	}
	return m
}

func savingsPlanRateFrom(service string, r sptypes.SavingsPlanOfferingRate) (PriceRate, bool) {
	usageType := ptrStr(r.UsageType)
	props := savingsPlanProperties(r.Properties)
	if service == "ecs" {
		return ecsSavingsPlanRate(r, usageType)
	}
	return instanceSavingsPlanRate(service, r, usageType, props)
}

// instanceSavingsPlanRate handles ec2/rds/elasticache. It excludes the same
// long-tail dimensions the On-Demand/RI path excludes (dedicated
// tenancy/hosts, unused capacity accounting rows, bundled SQL Server
// licensing) plus one SP-specific exclusion confirmed against live data:
// ElastiCache Serverless processing-unit rows (not a node rate). Aurora rows
// are included; spInstanceType (below) already works around their
// unreliable instanceType property.
func instanceSavingsPlanRate(service string, r sptypes.SavingsPlanOfferingRate, usageType string, props map[string]string) (PriceRate, bool) {
	if containsAny(usageType, "Dedicated", "Unused", "Host") {
		return PriceRate{}, false
	}
	productDescription := props["productDescription"]
	if strings.Contains(productDescription, "SQL Server") {
		return PriceRate{}, false
	}
	if service == "elasticache" && !strings.Contains(usageType, "NodeUsage") {
		return PriceRate{}, false
	}
	if r.SavingsPlanOffering == nil {
		return PriceRate{}, false
	}
	price, ok := parseUSD(ptrStr(r.Rate))
	if !ok {
		return PriceRate{}, false
	}

	offering := r.SavingsPlanOffering
	instanceType := spInstanceType(props["instanceType"], usageType)
	label := joinNonEmpty(" / ", instanceType, productDescription)
	attrs := map[string]string{}
	setIfNonEmpty(attrs, "instance_type", instanceType)
	setIfNonEmpty(attrs, "instance_family", instanceFamily(instanceType))
	if service == "ec2" {
		setIfNonEmpty(attrs, "os", productDescription)
	} else {
		setIfNonEmpty(attrs, "engine", productDescription)
	}

	lease := leaseFromDuration(offering.DurationSeconds)
	payment := string(offering.PaymentOption)
	return PriceRate{
		// offeringId + usageType だけでは一意にならない (issue 0049):
		// Savings Plans は特定の OS/engine に縛られず柔軟に適用される契約のため、
		// 同じ offeringId + usageType (instance_type) に対して productDescription
		// (RDS の engine・EC2/ElastiCache の OS 相当) だけが異なる複数行を返す。
		RateID:     ptrStr(offering.OfferingId) + "#" + usageType + "#" + productDescription,
		Model:      "savings_plan",
		Group:      spGroup(offering.PlanType),
		Label:      label,
		Attributes: attrs,
		Term: PriceTerm{
			Lease:   strPtr(lease),
			Payment: strPtr(payment),
		},
		// Savings Plans の前払い額は購入時のコミット金額に依存し、レート照会 API
		// (本関数の入力) には金額として現れない (RI と異なり固定カタログ価格が
		// 存在しない)。No Upfront では upfront_usd=0 は正確な値だが、Partial/All
		// Upfront でも同じく 0 を返すしかない点はフロントの見積もり UI 側で
		// 明示する (lib/pricingEstimate.ts 参照)。
		Unit:       "Hrs",
		PriceUSD:   price,
		UpfrontUSD: 0,
		Currency:   "USD",
		Operation:  ptrStr(r.Operation),
	}, true
}

// ecsSavingsPlanRate の RateID は offeringId + usageType のみで構成する (productDescription
// を含めない)。ECS Fargate は instanceSavingsPlanRate (ec2/rds/elasticache) と異なり、
// OS/architecture の別軸が productDescription ではなく usageType 文字列自体に "Windows"/"ARM"
// として埋め込まれる (下の os/arch 判定参照) ため、offeringId + usageType の組で既に一意になる
// (issue 0049 の調査で実データ上重複が確認されなかったことと整合する)。
func ecsSavingsPlanRate(r sptypes.SavingsPlanOfferingRate, usageType string) (PriceRate, bool) {
	unit, ok := fargateUnitFromUsageType(usageType)
	if !ok {
		return PriceRate{}, false
	}
	if r.SavingsPlanOffering == nil {
		return PriceRate{}, false
	}
	price, ok := parseUSD(ptrStr(r.Rate))
	if !ok {
		return PriceRate{}, false
	}

	offering := r.SavingsPlanOffering
	os := "Linux"
	if strings.Contains(usageType, "Windows") {
		os = "Windows"
	}
	arch := "x86"
	if strings.Contains(usageType, "ARM") {
		arch = "ARM"
	}
	label, attrs := fargateLabelAndAttributes(unit, os, arch)

	lease := leaseFromDuration(offering.DurationSeconds)
	payment := string(offering.PaymentOption)
	return PriceRate{
		RateID:     ptrStr(offering.OfferingId) + "#" + usageType,
		Model:      "savings_plan",
		Group:      spGroup(offering.PlanType),
		Label:      label,
		Attributes: attrs,
		Term: PriceTerm{
			Lease:   strPtr(lease),
			Payment: strPtr(payment),
		},
		Unit:       "Hrs",
		PriceUSD:   price,
		UpfrontUSD: 0,
		Currency:   "USD",
	}, true
}

func spGroup(planType sptypes.SavingsPlanType) string {
	switch planType {
	case sptypes.SavingsPlanTypeCompute:
		return "Compute Savings Plans"
	case sptypes.SavingsPlanTypeEc2Instance:
		return "EC2 Instance Savings Plans"
	case sptypes.SavingsPlanTypeDatabase:
		return "Database Savings Plans"
	default:
		return string(planType) + " Savings Plans"
	}
}

// leaseFromDuration converts a Savings Plans offering duration in seconds to
// the same "1yr"/"3yr" vocabulary Reserved Instances use. Savings Plans
// terms are exactly 1 or 3 years (confirmed against live data: 31536000s and
// 94608000s); rounding to the nearest year tolerates any leap-year-based
// variation without hardcoding the exact second counts.
func leaseFromDuration(seconds int64) string {
	years := int(math.Round(float64(seconds) / (365 * 24 * 3600)))
	return fmt.Sprintf("%dyr", years)
}

// spInstanceType returns the instance/node type for a Savings Plans rate.
// AWS's instanceType property is unreliable for some rows (confirmed against
// live RDS data: certain Aurora/Multi-AZ rows return just the instance
// family in uppercase, e.g. "R7G", dropping the size suffix entirely) — a
// well-formed instance type always contains a dot (e.g. "db.r7g.4xlarge"),
// so a property value without one is treated as broken and the usageType
// suffix (e.g. "db.r7g.4xl", an abbreviated but reliably-present form) is
// used instead.
func spInstanceType(property, usageType string) string {
	if property != "" && strings.Contains(property, ".") {
		return property
	}
	if idx := strings.LastIndex(usageType, ":"); idx >= 0 {
		return usageType[idx+1:]
	}
	return property
}

// ---- EC2 Spot (ec2:DescribeSpotPriceHistory) ----

// ec2SpotLookbackWindow bounds how far back DescribeSpotPriceHistory looks
// for the "current" snapshot. Spot price history entries are recorded only
// when a zone's price changes, so StartTime=now would return nothing for
// zones whose price hasn't changed in that instant; leaving StartTime unset
// instead pages through up to 90 days of history (see issue 0056 background:
// "直近約90日分の時系列を全ページ走査する" — too slow/costly for a live
// request). One hour is a pragmatic middle ground: issue 0060 の実 AWS 確認
// (ap-northeast-1) では 1 時間窓で 18000 件超・1141 種のインスタンスタイプ
// (m5/c5/r5 系だけで 153 種) が取得でき、主要タイプの欠落は無かった。窓を
// 広げる必要が生じたらここを調整する。
const ec2SpotLookbackWindow = 1 * time.Hour

// getEC2SpotPricing fetches and normalizes EC2 Spot rates. Unlike
// getResourcePricing/getSavingsPlanPricing, callers must not route this
// through pricecache.Load/Save (see handlers_pricing.go): Spot is always a
// live fetch, only deduped via pricecache.Fetch's singleflight.
func getEC2SpotPricing(ctx context.Context, client ec2SpotAPI, region string) (*PriceTable, error) {
	rates, err := fetchEC2SpotRates(ctx, client, region)
	if err != nil {
		return nil, fmt.Errorf("fetch ec2 spot pricing: %w", err)
	}
	sortPriceRates(rates)
	return &PriceTable{
		Service: EC2SpotService,
		Region:  region,
		Rates:   rates,
	}, nil
}

// fetchEC2SpotRates aggregates DescribeSpotPriceHistory rows into one rate
// per (instance_type, os) pair, taking the minimum price seen across
// Availability Zones (issue 0056 design decision: v1 shows a region-level
// representative value, not a per-zone row, to keep row counts in line with
// the existing On-Demand/Reserved table and avoid an AZ dimension the rest
// of the pricing table doesn't have). The RateID is built from instance_type
// and os only (no zone), which stays unique post-aggregation.
func fetchEC2SpotRates(ctx context.Context, client ec2SpotAPI, region string) ([]PriceRate, error) {
	type spotKey struct{ instanceType, os string }
	minPrice := make(map[spotKey]float64)

	startTime := time.Now().UTC().Add(-ec2SpotLookbackWindow)
	var next *string
	for {
		out, err := client.DescribeSpotPriceHistory(ctx, &ec2.DescribeSpotPriceHistoryInput{
			StartTime: aws.Time(startTime),
			NextToken: next,
		})
		if err != nil {
			return nil, fmt.Errorf("describe spot price history: %w", err)
		}
		for _, sp := range out.SpotPriceHistory {
			price, ok := parseUSD(ptrStr(sp.SpotPrice))
			if !ok {
				continue
			}
			instanceType := string(sp.InstanceType)
			os := spotOSFromProductDescription(sp.ProductDescription)
			if instanceType == "" || os == "" {
				continue
			}
			k := spotKey{instanceType, os}
			if cur, ok := minPrice[k]; !ok || price < cur {
				minPrice[k] = price
			}
		}
		// fetchSavingsPlans で確認した「最終ページで NextToken が nil ではなく空
		// 文字列になる」API の揺れに備え、こちらも空文字列を終端として扱う。
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		next = out.NextToken
	}

	rates := make([]PriceRate, 0, len(minPrice))
	for k, price := range minPrice {
		attrs := map[string]string{}
		setIfNonEmpty(attrs, "instance_type", k.instanceType)
		setIfNonEmpty(attrs, "instance_family", instanceFamily(k.instanceType))
		setIfNonEmpty(attrs, "os", k.os)
		rates = append(rates, PriceRate{
			RateID:     k.instanceType + "#" + k.os,
			Model:      "spot",
			Group:      "Spot",
			Label:      joinNonEmpty(" / ", k.instanceType, k.os),
			Attributes: attrs,
			Term:       PriceTerm{},
			Unit:       "Hrs",
			PriceUSD:   price,
			UpfrontUSD: 0,
			Currency:   "USD",
		})
	}
	return rates, nil
}

// spotOSFromProductDescription normalizes DescribeSpotPriceHistory's
// RIProductDescription into the On-Demand "operatingSystem" vocabulary
// (Linux/RHEL/SUSE/Windows/Ubuntu Pro), so the os attribute chip matches
// across On-Demand/Reserved and Spot rows (issue 0056) instead of splitting
// into e.g. "Linux" and "Linux/UNIX (Amazon VPC)". The API's documented
// filter values are four base descriptions (Linux/UNIX, Red Hat Enterprise
// Linux, SUSE Linux, Windows) each with an "(Amazon VPC)" variant; the VPC
// suffix carries no pricing-relevant distinction here and is stripped before
// mapping. ec2types.RIProductDescription's Go enum only declares the
// Linux/UNIX and Windows consts, but the API is documented to also return
// the RHEL/SUSE variants as plain strings, so this switches on the raw
// string rather than the (non-exhaustive) enum consts.
//
// issue 0060 の実 AWS 確認 (ap-northeast-1) で、ドキュメント記載の 4 系統に
// 加えて "Ubuntu Pro Linux" が返ることを確認した。On-Demand 側 (Pricing API
// の operatingSystem) はこれを "Ubuntu Pro" と表記するため、チップを揃える
// べく同じ値へ正規化する。未知の記述は base をそのまま返し、チップから
// 欠落させない (欠落より生の値を出す方が調査可能)。
func spotOSFromProductDescription(pd ec2types.RIProductDescription) string {
	base := strings.TrimSuffix(string(pd), " (Amazon VPC)")
	switch base {
	case "Linux/UNIX":
		return "Linux"
	case "Red Hat Enterprise Linux":
		return "RHEL"
	case "SUSE Linux":
		return "SUSE"
	case "Windows":
		return "Windows"
	case "Ubuntu Pro Linux":
		return "Ubuntu Pro"
	default:
		return base
	}
}

// ---- shared helpers ----

func parseUSD(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func joinNonEmpty(sep string, parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, sep)
}

func setIfNonEmpty(m map[string]string, key, value string) {
	if value != "" {
		m[key] = value
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func strPtr(s string) *string { return &s }
