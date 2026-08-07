package aws

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// mockSSMDescribeInstanceInformationClient は listSSMOnlineInstanceIDs が要求する
// ssm.DescribeInstanceInformationAPIClient をテスト用に実装する手書きモック。
// 受け取った Input を呼び出し順に記録し、用意したページを順に返す。
// 呼び出しはページネータ経由で逐次行われ、ページネータは呼び出しごとに Input を値でコピーして
// NextToken と MaxResults だけ差し替えるため、記録した後に内容が書き換わることはなく、
// ポインタのまま記録してよい。
type mockSSMDescribeInstanceInformationClient struct {
	pages  []*ssm.DescribeInstanceInformationOutput
	inputs []*ssm.DescribeInstanceInformationInput
}

func (m *mockSSMDescribeInstanceInformationClient) DescribeInstanceInformation(_ context.Context, params *ssm.DescribeInstanceInformationInput, _ ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error) {
	m.inputs = append(m.inputs, params)
	idx := len(m.inputs) - 1
	if idx >= len(m.pages) {
		return nil, fmt.Errorf("unexpected DescribeInstanceInformation call %d: %w", idx+1, errSSMNoMorePages)
	}
	return m.pages[idx], nil
}

// errSSMNoMorePages は用意したページを使い切った後の呼び出しでモックが返すエラー。
// 想定外の呼び出しを知らせると同時に、エラー伝播の検証で errors.Is の対象にも使う。
var errSSMNoMorePages = errors.New("no more pages prepared")

// TestListSSMOnlineInstanceIDsSendsPingStatusAndResourceTypeFilters は
// DescribeInstanceInformation へ Filters に PingStatus=Online と ResourceType=EC2Instance を
// 載せて送ることを検証する。この設定が無くても呼び出しは成功するが、関数名とコメントが示す
// 「Session Manager で接続可能な EC2 インスタンスのみ返す」という意味が壊れ、オフラインの
// インスタンスや EC2 以外のリソースタイプが静かに混入する。
// 期待値のキーと値は実装の式を経由せず AWS API の値で直接書き下している。
func TestListSSMOnlineInstanceIDsSendsPingStatusAndResourceTypeFilters(t *testing.T) {
	client := &mockSSMDescribeInstanceInformationClient{
		// 2 ページに分け、ページ送り後の呼び出しでも Filters が維持されることを確かめる。
		pages: []*ssm.DescribeInstanceInformationOutput{
			{
				InstanceInformationList: []ssmtypes.InstanceInformation{
					{InstanceId: aws.String("i-0aaaaaaaaaaaaaaaa")},
					// InstanceId が nil の要素は結果から落とされる。
					{InstanceId: nil},
				},
				NextToken: aws.String("page-2"),
			},
			{
				InstanceInformationList: []ssmtypes.InstanceInformation{
					{InstanceId: aws.String("i-0bbbbbbbbbbbbbbbb")},
				},
			},
		},
	}

	got, err := listSSMOnlineInstanceIDs(context.Background(), client)
	if err != nil {
		t.Fatalf("listSSMOnlineInstanceIDs() error = %v", err)
	}

	// Input 全体を比較する。Filters の内容と順序に加えて、期待していないフィールドが新たに
	// 設定される変更も検出できる。スライス全体を比較しているため呼び出し回数も同時に固定され、
	// 2 要素目の NextToken でページ送りが行われたことも固定される。
	// ページネータは Limit 未指定のため MaxResults を設定しない。
	filters := []ssmtypes.InstanceInformationStringFilter{
		{Key: aws.String("PingStatus"), Values: []string{"Online"}},
		{Key: aws.String("ResourceType"), Values: []string{"EC2Instance"}},
	}
	want := []*ssm.DescribeInstanceInformationInput{
		{Filters: filters},
		{Filters: filters, NextToken: aws.String("page-2")},
	}
	// SSM の Input と Filter は unexported フィールド (noSmithyDocumentSerde) を持つため無視する。
	opts := cmpopts.IgnoreUnexported(
		ssm.DescribeInstanceInformationInput{},
		ssmtypes.InstanceInformationStringFilter{},
	)
	if diff := cmp.Diff(want, client.inputs, opts); diff != "" {
		t.Errorf("DescribeInstanceInformation inputs mismatch (-want +got):\n%s", diff)
	}

	// 両ページのインスタンス ID が集約され、InstanceId が nil の要素が落とされることも確認する。
	wantIDs := []string{"i-0aaaaaaaaaaaaaaaa", "i-0bbbbbbbbbbbbbbbb"}
	if diff := cmp.Diff(wantIDs, got); diff != "" {
		t.Errorf("instance IDs mismatch (-want +got):\n%s", diff)
	}
}

// TestListSSMOnlineInstanceIDsPropagatesPageError は途中のページ取得が失敗したときに、
// エラーを握り潰さず、取得済みの部分的な結果も返さないことを検証する。
// エラーの文言そのものは固定せず、%w によるラップが維持されることだけを見る。
func TestListSSMOnlineInstanceIDsPropagatesPageError(t *testing.T) {
	// 1 ページ目は NextToken を返すが 2 ページ目を用意しない。ページネータは 2 回目の
	// 呼び出しを行い、モックがエラーを返す。
	client := &mockSSMDescribeInstanceInformationClient{
		pages: []*ssm.DescribeInstanceInformationOutput{
			{
				InstanceInformationList: []ssmtypes.InstanceInformation{
					{InstanceId: aws.String("i-0aaaaaaaaaaaaaaaa")},
				},
				NextToken: aws.String("page-2"),
			},
		},
	}

	got, err := listSSMOnlineInstanceIDs(context.Background(), client)
	if !errors.Is(err, errSSMNoMorePages) {
		t.Fatalf("listSSMOnlineInstanceIDs() error = %v, want error wrapping %v", err, errSSMNoMorePages)
	}
	// 1 ページ目の取得に成功していても部分的な結果は返さない。
	if got != nil {
		t.Errorf("listSSMOnlineInstanceIDs() = %v, want nil", got)
	}
}
