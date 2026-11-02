package api

import (
	"sync"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/accesslog"
)

// accessLogJobTTL は集計ジョブを保持する時間である。
const accessLogJobTTL = 10 * time.Minute

// accessLogJobSource はジョブに含まれる 1 つのソースの状態を表す。
type accessLogJobSource struct {
	SourceID        string
	Name            string
	Type            accesslog.SourceType
	Engine          accesslog.Engine
	QueryRegion     string
	OriginOf        string
	State           accesslog.QueryState
	ExecutionID     string
	LogGroup        string
	SourceKeyToID   map[string]string
	DefaultSourceID string
	BytesScanned    int64
	RecordsScanned  int64
	RecordsMatched  int64
	Rows            []accesslog.PairRow
	IPs             []accesslog.IPSummary
	Loaded          bool
	Err             string
}

// accessLogJob は 1 回の集計のジョブを表す。
type accessLogJob struct {
	mu        sync.Mutex
	ID        string
	Profile   string
	Region    string
	View      string
	CreatedAt time.Time
	Sources   []*accessLogJobSource
	Warnings  []string
}

// accessLogJobStore は集計ジョブをメモリに保持する。
// データベースを持たないため、API サーバの再起動でジョブは失われる。
type accessLogJobStore struct {
	mu   sync.Mutex
	jobs map[string]*accessLogJob
	ttl  time.Duration
	now  func() time.Time
}

func newAccessLogJobStore() *accessLogJobStore {
	return &accessLogJobStore{
		jobs: make(map[string]*accessLogJob),
		ttl:  accessLogJobTTL,
		now:  time.Now,
	}
}

func (s *accessLogJobStore) create(job *accessLogJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
}

// get はジョブを返す。期限切れの場合は削除して expired=true を返す。
func (s *accessLogJobStore) get(id string) (job *accessLogJob, expired bool, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok = s.jobs[id]
	if !ok {
		return nil, false, false
	}
	if s.now().Sub(job.CreatedAt) > s.ttl {
		delete(s.jobs, id)
		return job, true, false
	}
	return job, false, true
}

// cleanup は期限切れのジョブを削除して返す。呼び出し側が実行中のクエリを止める。
func (s *accessLogJobStore) cleanup() []*accessLogJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	var expired []*accessLogJob
	for id, job := range s.jobs {
		if now.Sub(job.CreatedAt) > s.ttl {
			expired = append(expired, job)
			delete(s.jobs, id)
		}
	}
	return expired
}
