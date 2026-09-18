package api

import (
	"context"
	"log/slog"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// ec2SampleTimeout は 1 組のサンプリング (一覧の取得) に許す時間。
// 遅い region があっても次の組の処理を待たせ続けないための上限である。
const ec2SampleTimeout = 30 * time.Second

// runEC2CountSampler は記録済みの profile と region の組について、interval ごとに
// EC2 の一覧を取得して Running インスタンス数を記録する。
//
// 対象は handleEC2 が記録を作った組に限る。全 profile と全 region を回すと、利用者が
// 見ていないアカウントとリージョンへの呼び出しが大半を占め、SSO が切れた profile の
// 失敗でログが埋まるためである。ctx のキャンセルで終了する。
func (s *Server) runEC2CountSampler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sampleEC2Counts(ctx)
		}
	}
}

// sampleEC2Counts は 1 周期分のサンプリングを行う。組ごとの失敗は記録せずに次の組へ
// 進み、失敗しても次の周期は続ける (取得できなかった時刻を 0 や前回値で補わない)。
func (s *Server) sampleEC2Counts(ctx context.Context) {
	for _, target := range s.ec2Counts.Keys() {
		// キャンセル済みなら残りの組を処理しない。WithTimeout はキャンセル済みの親から
		// 派生しても即座にキャンセルされるが、Keys() を回す分の無駄を避ける。
		if ctx.Err() != nil {
			return
		}
		resources, err := s.listEC2WithTimeout(ctx, target.Profile, target.Region)
		if err != nil {
			slog.Warn("ec2 count sample failed", "profile", target.Profile, "region", target.Region, "err", err)
			continue
		}
		s.ec2Counts.Record(target.Profile, target.Region, awsinternal.CountRunningEC2(resources), time.Now())
	}
}

// listEC2WithTimeout は 1 組分の一覧取得に上限時間を設けて呼ぶ。サンプリングの結果は
// 一覧のキャッシュには書かない (一覧の鮮度とキャッシュヘッダの意味を変えないため)。
func (s *Server) listEC2WithTimeout(ctx context.Context, profile, region string) ([]awsinternal.EC2Resource, error) {
	sampleCtx, cancel := context.WithTimeout(ctx, ec2SampleTimeout)
	defer cancel()
	return s.ec2Resources(sampleCtx, profile, region)
}
