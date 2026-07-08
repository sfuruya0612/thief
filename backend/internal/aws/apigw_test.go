package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	apigwv2types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
)

func TestApigwFromRestApi(t *testing.T) {
	tests := []struct {
		name    string
		in      apigwtypes.RestApi
		region  string
		wantID  string
		wantURL string
		wantTyp string
	}{
		{
			name:    "with region",
			in:      apigwtypes.RestApi{Id: aws.String("abc"), Name: aws.String("n")},
			region:  "ap-northeast-1",
			wantID:  "abc",
			wantURL: "https://abc.execute-api.ap-northeast-1.amazonaws.com",
			wantTyp: "REST",
		},
		{
			name:    "no region",
			in:      apigwtypes.RestApi{Id: aws.String("abc")},
			region:  "",
			wantURL: "",
			wantTyp: "REST",
			wantID:  "abc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apigwFromRestApi(tt.in, tt.region)
			if got.ID != tt.wantID || got.Endpoint != tt.wantURL || got.Type != tt.wantTyp || got.State != "active" {
				t.Errorf("got %#v", got)
			}
		})
	}
}

func TestApigwFromV2Api(t *testing.T) {
	tests := []struct {
		name    string
		in      apigwv2types.Api
		wantTyp string
	}{
		{"http", apigwv2types.Api{ApiId: aws.String("a"), Name: aws.String("n"), ProtocolType: apigwv2types.ProtocolTypeHttp, ApiEndpoint: aws.String("https://x")}, "HTTP"},
		{"ws", apigwv2types.Api{ApiId: aws.String("a"), Name: aws.String("n"), ProtocolType: apigwv2types.ProtocolTypeWebsocket}, "WebSocket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apigwFromV2Api(tt.in)
			if got.Type != tt.wantTyp || got.State != "active" || got.ID != "a" {
				t.Errorf("got %#v", got)
			}
		})
	}
}
