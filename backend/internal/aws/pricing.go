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
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	sptypes "github.com/aws/aws-sdk-go-v2/service/savingsplans/types"
)

// PriceTable is a normalized price rate table for one service/region,
// combining On-Demand, Reserved Instance, and Savings Plans rates.
type PriceTable struct {
	Service       string      `json:"service"`
	Region        string      `json:"region"`
	FetchedAt     time.Time   `json:"fetched_at"`
	Partial       bool        `json:"partial"`
	MissingModels []string    `json:"missing_models"`
	Rates         []PriceRate `json:"rates"`
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

// pricingServiceSpec maps a thief pricing service slug to the AWS Price
// List service code and Savings Plans identifiers needed to fetch its rates.
type pricingServiceSpec struct {
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
	spServiceCode sptypes.SavingsPlanRateServiceCode
	spPlanTypes   []sptypes.SavingsPlanType
}

// pricingServiceSpecs is the fixed allowlist of supported pricing services,
// matching the RI/SP support matrix confirmed against live AWS data (see
// issue 0045): EC2 has both RI and SP (Compute + EC2 Instance); RDS/
// ElastiCache have RI and Database SP only; ECS (Fargate) has no RI, only
// Compute SP.
var pricingServiceSpecs = map[string]pricingServiceSpec{
	"ec2": {
		awsServiceCode: "AmazonEC2",
		productFamily:  "Compute Instance",
		riSupported:    true,
		spServiceCode:  sptypes.SavingsPlanRateServiceCodeEc2,
		spPlanTypes:    []sptypes.SavingsPlanType{sptypes.SavingsPlanTypeCompute, sptypes.SavingsPlanTypeEc2Instance},
	},
	"rds": {
		awsServiceCode: "AmazonRDS",
		productFamily:  "Database Instance",
		riSupported:    true,
		spServiceCode:  sptypes.SavingsPlanRateServiceCodeRds,
		spPlanTypes:    []sptypes.SavingsPlanType{sptypes.SavingsPlanTypeDatabase},
	},
	"elasticache": {
		awsServiceCode: "AmazonElastiCache",
		productFamily:  "Cache Instance",
		riSupported:    true,
		spServiceCode:  sptypes.SavingsPlanRateServiceCodeElasticache,
		spPlanTypes:    []sptypes.SavingsPlanType{sptypes.SavingsPlanTypeDatabase},
	},
	"ecs": {
		awsServiceCode: "AmazonECS",
		productFamily:  "Compute",
		riSupported:    false,
		spServiceCode:  sptypes.SavingsPlanRateServiceCodeFargate,
		spPlanTypes:    []sptypes.SavingsPlanType{sptypes.SavingsPlanTypeCompute},
	},
}

// ValidatePricingService returns ErrInvalidPricingService unless service is
// one of the supported pricing service slugs (ec2/rds/elasticache/ecs).
func ValidatePricingService(service string) error {
	if _, ok := pricingServiceSpecs[service]; !ok {
		return fmt.Errorf("%w: %q", ErrInvalidPricingService, service)
	}
	return nil
}

// GetPricing returns the normalized price table for service/region, using
// profile's credentials to call the Price List and Savings Plans APIs.
// Price List/Savings Plans endpoints are global services; both clients are
// pinned to us-east-1 (mirroring newCostExplorerClient), and region is used
// only as an API-side filter value, not a client region.
func GetPricing(ctx context.Context, profile, region, service string) (*PriceTable, error) {
	spec, ok := pricingServiceSpecs[service]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidPricingService, service)
	}
	pricingClient, err := newPricingClient(ctx, profile)
	if err != nil {
		return nil, err
	}
	spClient, err := newSavingsPlansClient(ctx, profile)
	if err != nil {
		return nil, err
	}
	return getPricing(ctx, pricingClient, spClient, region, service, spec)
}

func getPricing(ctx context.Context, pc pricingAPI, sp savingsPlansAPI, region, service string, spec pricingServiceSpec) (*PriceTable, error) {
	var (
		onDemandRates []PriceRate
		onDemandErr   error
		spRates       []PriceRate
		spErr         error
		wg            sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		onDemandRates, onDemandErr = fetchOnDemandAndReserved(ctx, pc, region, service, spec)
	}()
	go func() {
		defer wg.Done()
		spRates, spErr = fetchSavingsPlans(ctx, sp, region, service, spec)
	}()
	wg.Wait()

	// 取得の完全性: On-Demand/RI が失敗した表は破棄してエラーを返す
	// (呼び出し側はこの表をキャッシュに書き込まない)。
	if onDemandErr != nil {
		return nil, fmt.Errorf("fetch on-demand/reserved pricing for %s: %w", service, onDemandErr)
	}

	table := &PriceTable{
		Service:       service,
		Region:        region,
		Partial:       false,
		MissingModels: []string{},
		Rates:         onDemandRates,
	}
	if spErr != nil {
		// SP のみの失敗は On-Demand/RI の表を活かして縮退させる。不完全さは
		// partial/missing_models で明示し、「完全」として偽装しない。
		slog.Warn("fetch savings plans pricing failed; degrading to on-demand/reserved only",
			"service", service, "region", region, "err", spErr)
		table.Partial = true
		table.MissingModels = []string{"savings_plan"}
	} else {
		table.Rates = append(table.Rates, spRates...)
	}
	sortPriceRates(table.Rates)
	return table, nil
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

func fetchOnDemandAndReserved(ctx context.Context, client pricingAPI, region, service string, spec pricingServiceSpec) ([]PriceRate, error) {
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
	var next *string
	for {
		out, err := client.GetProducts(ctx, &pricing.GetProductsInput{
			ServiceCode: aws.String(spec.awsServiceCode),
			Filters:     filters,
			NextToken:   next,
		})
		if err != nil {
			return nil, fmt.Errorf("get products: %w", err)
		}
		for _, raw := range out.PriceList {
			doc, perr := parsePriceDocument(raw)
			if perr != nil {
				slog.Warn("skip malformed price list document", "service", service, "err", perr)
				continue
			}
			rates = append(rates, priceRatesFromDocument(service, spec, *doc)...)
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
	return rates, nil
}

func priceRatesFromDocument(service string, spec pricingServiceSpec, doc priceListDocument) []PriceRate {
	// GetProducts の productFamily フィルタ (fetchOnDemandAndReserved) が API 側
	// 不具合等で効かなかった場合の保険として、パース後にも二重チェックする。
	if doc.Product.ProductFamily != spec.productFamily {
		return nil
	}
	if service == "ecs" {
		return ecsOnDemandRatesFromDocument(doc)
	}
	return instanceOnDemandRatesFromDocument(service, spec, doc)
}

func instanceOnDemandRatesFromDocument(service string, spec pricingServiceSpec, doc priceListDocument) []PriceRate {
	attrs := doc.Product.Attributes
	if strings.Contains(attrs["usagetype"], "ExtendedSupport") {
		// EOL 延長サポート課金 (RDS/ElastiCache の一部エンジンに存在) は同一
		// instanceType で複数の紛らわしい重複行を生むため v1 スコープ外とする。
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
		return joinNonEmpty(" / ", attrs["instanceType"], attrs["operatingSystem"], attrs["tenancy"])
	case "rds":
		storageLabel := "Standard"
		if rdsStorageType(attrs["storage"]) == "io_optimized" {
			storageLabel = "IO-Optimized"
		}
		return joinNonEmpty(" / ", attrs["instanceType"], attrs["databaseEngine"], attrs["deploymentOption"], storageLabel)
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
		setIfNonEmpty(out, "os", attrs["operatingSystem"])
		setIfNonEmpty(out, "tenancy", attrs["tenancy"])
	case "rds":
		setIfNonEmpty(out, "instance_type", attrs["instanceType"])
		setIfNonEmpty(out, "engine", attrs["databaseEngine"])
		setIfNonEmpty(out, "deployment_option", attrs["deploymentOption"])
		setIfNonEmpty(out, "license_model", attrs["licenseModel"])
		out["storage_type"] = rdsStorageType(attrs["storage"])
	case "elasticache":
		setIfNonEmpty(out, "instance_type", attrs["instanceType"])
		setIfNonEmpty(out, "engine", attrs["cacheEngine"])
	}
	return out
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

func fetchSavingsPlans(ctx context.Context, client savingsPlansAPI, region, service string, spec pricingServiceSpec) ([]PriceRate, error) {
	rates := []PriceRate{}
	var next *string
	for {
		out, err := client.DescribeSavingsPlansOfferingRates(ctx, &savingsplans.DescribeSavingsPlansOfferingRatesInput{
			SavingsPlanTypes: spec.spPlanTypes,
			ServiceCodes:     []sptypes.SavingsPlanRateServiceCode{spec.spServiceCode},
			Filters: []sptypes.SavingsPlanOfferingRateFilterElement{
				{Name: sptypes.SavingsPlanRateFilterAttributeRegion, Values: []string{region}},
			},
			NextToken: next,
		})
		if err != nil {
			return nil, fmt.Errorf("describe savings plans offering rates: %w", err)
		}
		for _, r := range out.SearchResults {
			if rate, ok := savingsPlanRateFrom(service, r); ok {
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
	return rates, nil
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
