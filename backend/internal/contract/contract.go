// Package contract は backend の API レスポンス構造体と frontend の Raw 型の契約を
// ゴールデン JSON で検証するための生成器を提供する。契約対象の型とゴールデンファイル名の
// 対応はレジストリ (Registry) が定め、フィラー (Fill) が全フィールドに決定的な非ゼロ値を
// 埋めたインスタンスを生成する。全フィールドに値を入れるのは、omitempty のフィールドや
// nil のスライスと map の脱落を防ぎ、ゴールデンのキー集合を固定するためである。
package contract

import (
	"fmt"
	"reflect"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/api"
	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// Entry は契約対象 1 型とゴールデンファイルの対応を表す。
type Entry struct {
	// Name はゴールデンファイル名 (拡張子抜き) に使う backend の型名。
	Name string
	// Value は契約対象の型のゼロ値。Fill がこの型のインスタンスを生成する。
	Value any
}

// Registry は契約対象 60 型の一覧である。issues/closed/0099 の対応表 (62 行) のうち
// frontend に Raw の対応を持つ 60 行を転記した。frontend に消費者が無い
// SSOAccountResource と SSMValueResponse は含めない。
var Registry = []Entry{
	{Name: "APIGatewayResource", Value: aws.APIGatewayResource{}},
	{Name: "AthenaCatalog", Value: aws.AthenaCatalog{}},
	{Name: "AthenaDatabase", Value: aws.AthenaDatabase{}},
	{Name: "AthenaWorkgroup", Value: aws.AthenaWorkgroup{}},
	{Name: "AthenaColumn", Value: aws.AthenaColumn{}},
	{Name: "AthenaTable", Value: aws.AthenaTable{}},
	{Name: "AthenaQueryExecution", Value: aws.AthenaQueryExecution{}},
	{Name: "AthenaResultColumn", Value: aws.AthenaResultColumn{}},
	{Name: "AthenaResultPage", Value: aws.AthenaResultPage{}},
	{Name: "CFNStackResource", Value: aws.CFNStackResource{}},
	{Name: "CFNStackDetail", Value: aws.CFNStackDetail{}},
	{Name: "CFNParameter", Value: aws.CFNParameter{}},
	{Name: "CFNOutput", Value: aws.CFNOutput{}},
	{Name: "CFNStackEvent", Value: aws.CFNStackEvent{}},
	{Name: "CFNStackResourceSummary", Value: aws.CFNStackResourceSummary{}},
	{Name: "CloudFrontResource", Value: aws.CloudFrontResource{}},
	{Name: "CloudFrontBehavior", Value: aws.CloudFrontBehavior{}},
	{Name: "LogGroupInfo", Value: aws.LogGroupInfo{}},
	{Name: "LogEventInfo", Value: aws.LogEventInfo{}},
	{Name: "LogEventPage", Value: aws.LogEventPage{}},
	{Name: "CostResource", Value: aws.CostResource{}},
	{Name: "ForecastResource", Value: aws.ForecastResource{}},
	{Name: "DynamoResource", Value: aws.DynamoResource{}},
	{Name: "DynamoKeyAttribute", Value: aws.DynamoKeyAttribute{}},
	{Name: "DynamoIndexSchema", Value: aws.DynamoIndexSchema{}},
	{Name: "DynamoTableSchema", Value: aws.DynamoTableSchema{}},
	{Name: "EC2Resource", Value: aws.EC2Resource{}},
	{Name: "ECRRepoResource", Value: aws.ECRRepoResource{}},
	{Name: "ECRImageResource", Value: aws.ECRImageResource{}},
	{Name: "ECSResource", Value: aws.ECSResource{}},
	{Name: "ECSServiceResource", Value: aws.ECSServiceResource{}},
	{Name: "ECSTaskResource", Value: aws.ECSTaskResource{}},
	{Name: "ECSTaskContainerDetail", Value: aws.ECSTaskContainerDetail{}},
	{Name: "ECSContainerResource", Value: aws.ECSContainerResource{}},
	{Name: "ElastiCacheResource", Value: aws.ElastiCacheResource{}},
	{Name: "ElastiCacheParameter", Value: aws.ElastiCacheParameter{}},
	{Name: "ELBResource", Value: aws.ELBResource{}},
	{Name: "ELBListenerResource", Value: aws.ELBListenerResource{}},
	{Name: "ELBRuleResource", Value: aws.ELBRuleResource{}},
	{Name: "ELBTargetGroupResource", Value: aws.ELBTargetGroupResource{}},
	{Name: "ELBTargetHealthResource", Value: aws.ELBTargetHealthResource{}},
	{Name: "IAMResource", Value: aws.IAMResource{}},
	{Name: "KinesisResource", Value: aws.KinesisResource{}},
	{Name: "LambdaResource", Value: aws.LambdaResource{}},
	{Name: "NATGatewayResource", Value: aws.NATGatewayResource{}},
	{Name: "PriceTable", Value: aws.PriceTable{}},
	{Name: "PriceRate", Value: aws.PriceRate{}},
	{Name: "PriceTerm", Value: aws.PriceTerm{}},
	{Name: "RDSResource", Value: aws.RDSResource{}},
	{Name: "RDSParameter", Value: aws.RDSParameter{}},
	{Name: "RDSClusterParameterGroup", Value: aws.RDSClusterParameterGroup{}},
	{Name: "RegionResource", Value: aws.RegionResource{}},
	{Name: "S3Resource", Value: aws.S3Resource{}},
	{Name: "S3ObjectResource", Value: aws.S3ObjectResource{}},
	{Name: "SecretResource", Value: aws.SecretResource{}},
	{Name: "SQSResource", Value: aws.SQSResource{}},
	{Name: "SSMParameterResource", Value: aws.SSMParameterResource{}},
	{Name: "WAFResource", Value: aws.WAFResource{}},
	{Name: "WAFRule", Value: aws.WAFRule{}},
	{Name: "ValueResponse", Value: api.ValueResponse{}},
}

// Tags は v の型の各エクスポートフィールドのフィールド名と json タグの一覧を、定義順の
// 決定的なテキスト行として返す。フィラーは全フィールドに非ゼロ値を入れるため、omitempty の
// 増減ではゴールデン JSON の出力が変わらない。omitempty を含むタグの変更そのものを検出する
// ために、タグの一覧を別のゴールデンとして比較する。ネストした構造体の型はレジストリに
// 単独の行を持つため、各型は自分のフィールドだけを列挙すれば全型が覆われる。
func Tags(v any) []string {
	t := reflect.TypeOf(v)
	lines := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %q", f.Name, f.Tag.Get("json")))
	}
	return lines
}

// fillTime はフィラーが time.Time のフィールドに使う固定値。乱数と現在時刻を使わないのは、
// 同一コードからの生成結果を常に一致させ、ゴールデンとの比較を成立させるためである。
var fillTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// Fill は v と同じ型のインスタンスを生成し、全フィールドに決定的な非ゼロ値を埋めて返す。
// 値はフィールドの型と走査順から導出する (走査順は構造体定義のフィールド順で固定される)。
// ネストした構造体は再帰し、スライスは 1 要素、map は 1 エントリ、ポインタは指す先を
// 生成して埋める。json:"-" のフィールドはエンコードに現れないため埋めずに飛ばす。
func Fill(v any) (any, error) {
	rv := reflect.New(reflect.TypeOf(v)).Elem()
	c := counter{}
	if err := fillValue(rv, &c, reflect.TypeOf(v).Name()); err != nil {
		return nil, err
	}
	return rv.Interface(), nil
}

// counter は走査順に増える連番で、文字列と数値の生成値を決定的に導出する。
type counter struct{ n int }

func (c *counter) next() int {
	c.n++
	return c.n
}

// fillValue は v に型に応じた非ゼロ値を設定する。path はエラーメッセージ用のフィールド経路。
func fillValue(v reflect.Value, c *counter, path string) error {
	switch v.Kind() {
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			v.Set(reflect.ValueOf(fillTime))
			return nil
		}
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			if f.Tag.Get("json") == "-" {
				continue
			}
			if err := fillValue(v.Field(i), c, path+"."+f.Name); err != nil {
				return err
			}
		}
	case reflect.String:
		v.SetString(fmt.Sprintf("s%d", c.next()))
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(c.next()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(c.next()))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(c.next()) + 0.5)
	case reflect.Slice:
		elem := reflect.New(v.Type().Elem()).Elem()
		if err := fillValue(elem, c, path+"[0]"); err != nil {
			return err
		}
		v.Set(reflect.Append(reflect.MakeSlice(v.Type(), 0, 1), elem))
	case reflect.Map:
		key := reflect.New(v.Type().Key()).Elem()
		if err := fillValue(key, c, path+"{key}"); err != nil {
			return err
		}
		val := reflect.New(v.Type().Elem()).Elem()
		if err := fillValue(val, c, path+"{value}"); err != nil {
			return err
		}
		m := reflect.MakeMapWithSize(v.Type(), 1)
		m.SetMapIndex(key, val)
		v.Set(m)
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		if err := fillValue(p.Elem(), c, path); err != nil {
			return err
		}
		v.Set(p)
	default:
		return fmt.Errorf("fill %s: unsupported kind %s", path, v.Kind())
	}
	return nil
}
