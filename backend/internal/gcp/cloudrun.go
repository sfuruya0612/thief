package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	locationpb "google.golang.org/genproto/googleapis/cloud/location"
)

// RunResourceInfo は Cloud Run のサービス / ジョブを 1 レコードに正規化した表現。
type RunResourceInfo struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Region     string `json:"region"`
	ProjectID  string `json:"project_id"`
	URI        string `json:"uri"`
	CreateTime string `json:"create_time"`
	UpdateTime string `json:"update_time"`
}

// ListCloudRun は指定プロジェクトの全ロケーションを横断して Cloud Run のサービスとジョブを列挙する。
// クライアントは呼び出し内で作成・破棄する。
func ListCloudRun(ctx context.Context, projectID string) ([]RunResourceInfo, error) {
	parent := "projects/" + projectID + "/locations/-"

	// WithQuotaProject を指定しない場合、ADC のデフォルト quota project がクオータ判定に
	// 使われ、選択中の projectID と食い違ってしまうため常に明示する。
	svcClient, err := run.NewServicesClient(ctx, option.WithQuotaProject(projectID))
	if err != nil {
		return nil, fmt.Errorf("create cloud run services client: %w", err)
	}
	defer svcClient.Close()

	jobClient, err := run.NewJobsClient(ctx, option.WithQuotaProject(projectID))
	if err != nil {
		return nil, fmt.Errorf("create cloud run jobs client: %w", err)
	}
	defer jobClient.Close()

	var resources []RunResourceInfo

	// 各フェーズの所要時間を計測してログに残す (issue 0041: 50 秒遅延の原因調査)。
	// 逐次処理 (ロケーションごとの ListJobs) と個々の API 応答のどちらが支配的かを
	// 実環境のログから切り分けられるようにする。
	overallStart := time.Now()

	svcStart := time.Now()
	svcIt := svcClient.ListServices(ctx, &runpb.ListServicesRequest{Parent: parent})
	for {
		s, err := svcIt.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterate cloud run services: %w", err)
		}
		r := runResourceFromService(s)
		r.ProjectID = projectID
		resources = append(resources, r)
	}
	slog.Info("cloud run list services done",
		"project_id", projectID, "duration_ms", time.Since(svcStart).Milliseconds(), "count", len(resources))

	// Jobs API は Services と異なり Parent に "-" ワイルドカードを受け付けないため、
	// ロケーションを列挙してからロケーションごとに ListJobs を呼び出す。
	locStart := time.Now()
	locations, err := listRunLocations(ctx, projectID)
	if err != nil {
		return nil, err
	}
	slog.Info("cloud run list locations done",
		"project_id", projectID, "duration_ms", time.Since(locStart).Milliseconds(), "count", len(locations))

	jobsStart := time.Now()
	jobCount := 0
	for _, location := range locations {
		locJobStart := time.Now()
		jobParent := "projects/" + projectID + "/locations/" + location
		jobIt := jobClient.ListJobs(ctx, &runpb.ListJobsRequest{Parent: jobParent})
		locJobCount := 0
		for {
			j, err := jobIt.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("iterate cloud run jobs in %s: %w", location, err)
			}
			r := runResourceFromJob(j)
			r.ProjectID = projectID
			resources = append(resources, r)
			locJobCount++
		}
		jobCount += locJobCount
		slog.Info("cloud run list jobs in location done",
			"project_id", projectID, "location", location,
			"duration_ms", time.Since(locJobStart).Milliseconds(), "count", locJobCount)
	}
	slog.Info("cloud run list jobs done (all locations, sequential)",
		"project_id", projectID, "duration_ms", time.Since(jobsStart).Milliseconds(),
		"locations", len(locations), "count", jobCount)

	slog.Info("cloud run list all done",
		"project_id", projectID, "duration_ms", time.Since(overallStart).Milliseconds(), "count", len(resources))

	return resources, nil
}

// listRunLocations は Cloud Run が利用可能なロケーション ID の一覧を返す。
func listRunLocations(ctx context.Context, projectID string) ([]string, error) {
	locClient, err := run.NewLocationsClient(ctx, option.WithQuotaProject(projectID))
	if err != nil {
		return nil, fmt.Errorf("create cloud run locations client: %w", err)
	}
	defer locClient.Close()

	var locations []string
	it := locClient.ListLocations(ctx, &locationpb.ListLocationsRequest{
		Name: "projects/" + projectID,
	})
	for {
		loc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list cloud run locations: %w", err)
		}
		locations = append(locations, loc.GetLocationId())
	}
	return locations, nil
}

// regionFromResourceName は Cloud Run のフルリソース名 (projects/{p}/locations/{loc}/services/{s} 等)
// から locations の次のセグメントを取り出す純関数。見つからない場合は空文字を返す。
func regionFromResourceName(name string) string {
	const key = "/locations/"
	i := strings.Index(name, key)
	if i < 0 {
		return ""
	}
	rest := name[i+len(key):]
	if j := strings.Index(rest, "/"); j >= 0 {
		return rest[:j]
	}
	return rest
}

func runResourceFromService(s *runpb.Service) RunResourceInfo {
	if s == nil {
		return RunResourceInfo{Kind: "service"}
	}
	return RunResourceInfo{
		Name:       s.GetName(),
		Kind:       "service",
		Region:     regionFromResourceName(s.GetName()),
		URI:        s.GetUri(),
		CreateTime: formatTimestamp(s.GetCreateTime().AsTime(), s.GetCreateTime() != nil),
		UpdateTime: formatTimestamp(s.GetUpdateTime().AsTime(), s.GetUpdateTime() != nil),
	}
}

func runResourceFromJob(j *runpb.Job) RunResourceInfo {
	if j == nil {
		return RunResourceInfo{Kind: "job"}
	}
	return RunResourceInfo{
		Name:       j.GetName(),
		Kind:       "job",
		Region:     regionFromResourceName(j.GetName()),
		CreateTime: formatTimestamp(j.GetCreateTime().AsTime(), j.GetCreateTime() != nil),
		UpdateTime: formatTimestamp(j.GetUpdateTime().AsTime(), j.GetUpdateTime() != nil),
	}
}

func formatTimestamp(t time.Time, present bool) string {
	if !present || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
