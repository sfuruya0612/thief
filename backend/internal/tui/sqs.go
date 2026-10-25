package tui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// sqsService は SQS キューの一覧を提供する Service。
type sqsService struct {
	list func(ctx context.Context, profile, region string) ([]aws.SQSResource, error)
}

// NewSQSService は SQS の Service を返す。list には aws.ListSQSResources を渡す。
func NewSQSService(list func(ctx context.Context, profile, region string) ([]aws.SQSResource, error)) Service {
	return &sqsService{list: list}
}

func (s *sqsService) ID() string    { return "sqs" }
func (s *sqsService) Title() string { return "SQS" }

// Columns は Web UI の sqsColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// Available は桁区切り (toLocaleString)、Retention は "<n>d"。Web UI に dash を描画するセルは無い。
func (s *sqsService) Columns() []Column {
	return []Column{
		{Title: "Queue"},
		{Title: "State"},
		{Title: "Type"},
		{Title: "Available"},
		{Title: "In flight"},
		{Title: "Retention"},
		{Title: "Region"},
	}
}

func (s *sqsService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list SQS queues: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, sqsRow{res: it, region: region})
	}
	return rows, nil
}

func (s *sqsService) SupportsSession() bool { return false }

// sqsRow はキュー 1 つの行。ID はキューの ARN (属性に無ければ URL)、Name は URL の末尾のキュー名で、
// どちらも一覧関数が決める。Type は Standard か FIFO。
type sqsRow struct {
	res    aws.SQSResource
	region string
}

func (r sqsRow) ID() string    { return r.res.ID }
func (r sqsRow) Name() string  { return r.res.Name }
func (r sqsRow) State() string { return r.res.State }

func (r sqsRow) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		r.res.Type,
		formatCount(int64(r.res.AvailableMessages)),
		strconv.Itoa(r.res.InFlight),
		strconv.Itoa(r.res.RetentionDays) + "d",
		r.region,
	}
}

// Overview は Web UI の sqsOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
// Retention は列の "<n>d" と違い "<n> days" で、Web UI の表記の違いをそのまま写す。
func (r sqsRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Type", Value: r.res.Type},
		{Key: "Messages available", Value: formatCount(int64(r.res.AvailableMessages))},
		{Key: "Messages in flight", Value: strconv.Itoa(r.res.InFlight)},
		{Key: "Retention", Value: strconv.Itoa(r.res.RetentionDays) + " days"},
		{Key: "Region", Value: r.region},
	}
}

func (r sqsRow) Tags() map[string]string { return r.res.Tags }

// TagsFetchFailed は ListSQSResources の内部の ListQueueTags が失敗したかを返す (TagsFetchFailer)。
func (r sqsRow) TagsFetchFailed() bool { return r.res.TagsFetchFailed }
