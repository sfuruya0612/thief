package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/accesslog"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

const (
	accessLogMaxSources       = 5
	accessLogMaxResults       = 200
	accessLogDefaultRange     = 24 * time.Hour
	accessLogMaxRange         = 7 * 24 * time.Hour
	accessLogInsightsMaxRange = 24 * time.Hour
)

type accessLogAthenaGroup struct {
	region          string
	catalog         string
	database        string
	table           string
	profile         accesslog.SchemaProfile
	columns         []string
	partitionKeys   []string
	partitionStyle  accesslog.PartitionStyle
	sourceKeyColumn string
	sources         []accesslog.AthenaSource
}

type accessLogInsightsGroup struct {
	region   string
	logGroup string
	sources  []accesslog.AthenaSource
}

func (s *Server) handleAccessLogSources(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("access-logs-sources", profile, region), time.Hour, writeAccessLogError, func() (any, error) {
		return s.accessLogs.discover(r.Context(), profile, region)
	})
}

func (s *Server) handleAccessLogTableValidate(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	var req AccessLogTableValidateRequest
	if err := jsonDecode(r, &req); err != nil {
		writeBadRequest(w, "invalid JSON body")
		return
	}
	if req.SourceID == "" || req.Database == "" || req.Table == "" {
		writeBadRequest(w, "source_id, database and table are required")
		return
	}
	if (req.Catalog != "" && !accesslog.ValidateIdent(req.Catalog)) ||
		!accesslog.ValidateIdent(req.Database) || !accesslog.ValidateIdent(req.Table) {
		writeAccessLogError(w, fmt.Errorf("%w: invalid catalog, database or table", accesslog.ErrInvalidTable))
		return
	}
	sources, err := s.accessLogSources(r.Context(), profile, region)
	if err != nil {
		writeAccessLogError(w, err)
		return
	}
	source, ok := findAccessLogSource(sources.Sources, req.SourceID)
	if !ok {
		writeAccessLogError(w, fmt.Errorf("%w: source %s", accesslog.ErrInvalidSource, req.SourceID))
		return
	}
	if source.Engine != accesslog.EngineAthena {
		writeAccessLogError(w, fmt.Errorf("%w: source %s does not use Athena", accesslog.ErrSourceNotReady, req.SourceID))
		return
	}
	meta, err := s.accessLogs.tableMetadata(r.Context(), profile, source.QueryRegion, req.Catalog, req.Database, req.Table, req.Workgroup)
	if err != nil {
		writeAccessLogError(w, accessLogTableError(err))
		return
	}
	columns := athenaColumnNames(meta.Columns)
	partitionKeys := athenaColumnNames(meta.PartitionKeys)
	profileName, ok := accesslog.DetectProfile(columns)
	validation := accesslog.TableValidation{
		Columns:       columns,
		PartitionKeys: partitionKeys,
	}
	if !ok {
		validation.Issues = append(validation.Issues, "cannot detect the Athena schema profile from the table columns")
		writeJSON(w, AccessLogTableValidateResponse{Validation: validation})
		return
	}
	validation.SchemaProfile = profileName
	validation.SourceKeyColumn = accesslog.FindSourceKeyColumn(profileName, columns)
	validation.Attribution = validation.SourceKeyColumn != ""
	validation.MissingColumns = accesslog.MissingColumns(profileName, columns)
	validation.Valid = len(validation.MissingColumns) == 0
	if len(validation.MissingColumns) > 0 {
		validation.Issues = append(validation.Issues, "missing columns: "+strings.Join(validation.MissingColumns, ", "))
	}
	if !validation.Attribution {
		validation.Issues = append(validation.Issues, "the table has no column that identifies the resource; the source cannot be combined with other sources")
	}
	ddl, ddlErr := accesslog.BuildDDL(accesslog.DDLInput{Source: source, Database: req.Database, Table: req.Table})
	if ddlErr != nil {
		validation.Issues = append(validation.Issues, "cannot generate DDL: "+ddlErr.Error())
	}
	writeJSON(w, AccessLogTableValidateResponse{Validation: validation, SetupSQL: ddl})
}

func (s *Server) handleAccessLogQueryStart(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	var req AccessLogQueryRequest
	if err := jsonDecode(r, &req); err != nil {
		writeBadRequest(w, "invalid JSON body")
		return
	}
	if req.View != "pairs" && req.View != "ips" {
		writeBadRequest(w, "view must be pairs or ips")
		return
	}
	if len(req.Sources) == 0 {
		writeBadRequest(w, "at least one source is required")
		return
	}
	if len(req.Sources) > accessLogMaxSources {
		writeBadRequest(w, fmt.Sprintf("at most %d sources can be selected", accessLogMaxSources))
		return
	}
	from, err := time.Parse(time.RFC3339, req.From)
	if err != nil {
		writeAccessLogError(w, fmt.Errorf("%w: from", accesslog.ErrInvalidTimeRange))
		return
	}
	to, err := time.Parse(time.RFC3339, req.To)
	if err != nil {
		writeAccessLogError(w, fmt.Errorf("%w: to", accesslog.ErrInvalidTimeRange))
		return
	}
	timeRange := accesslog.TimeRange{From: from.UTC(), To: to.UTC()}
	if err := timeRange.Validate(); err != nil {
		writeAccessLogError(w, err)
		return
	}
	maxRange := accessLogDefaultRange
	if req.ConfirmLongRange {
		maxRange = accessLogMaxRange
	}
	if timeRange.To.Sub(timeRange.From) > maxRange {
		writeAccessLogError(w, fmt.Errorf("%w: maximum range is %s", accesslog.ErrRangeTooLong, maxRange))
		return
	}
	if req.Limit <= 0 || req.Limit > accessLogMaxResults {
		req.Limit = accessLogMaxResults
	}

	discovered, err := s.accessLogSources(r.Context(), profile, region)
	if err != nil {
		writeAccessLogError(w, err)
		return
	}
	byID := make(map[string]accesslog.Source, len(discovered.Sources))
	for _, source := range discovered.Sources {
		byID[source.ID] = source
	}

	athenaGroups := make(map[string]*accessLogAthenaGroup)
	insightsGroups := make(map[string]*accessLogInsightsGroup)
	jobSources := make([]*accessLogJobSource, 0, len(req.Sources))

	for _, requested := range req.Sources {
		source, ok := byID[requested.ID]
		if !ok {
			writeAccessLogError(w, fmt.Errorf("%w: %s", accesslog.ErrInvalidSource, requested.ID))
			return
		}
		if !source.Status.LoggingEnabled || source.Engine == "" {
			writeAccessLogError(w, fmt.Errorf("%w: %s", accesslog.ErrSourceNotReady, source.Name))
			return
		}
		if source.Engine == accesslog.EngineLogsInsights && timeRange.To.Sub(timeRange.From) > accessLogInsightsMaxRange {
			writeAccessLogError(w, fmt.Errorf("%w: CloudWatch Logs sources are limited to %s", accesslog.ErrRangeTooLong, accessLogInsightsMaxRange))
			return
		}
		jobSource := newAccessLogJobSource(source)
		switch source.Engine {
		case accesslog.EngineAthena:
			if requested.Table == nil {
				writeBadRequest(w, "table is required for an Athena source")
				return
			}
			if (requested.Table.Catalog != "" && !accesslog.ValidateIdent(requested.Table.Catalog)) ||
				!accesslog.ValidateIdent(requested.Table.Database) || !accesslog.ValidateIdent(requested.Table.Table) {
				writeAccessLogError(w, fmt.Errorf("%w: invalid catalog, database or table", accesslog.ErrInvalidTable))
				return
			}
			meta, err := s.accessLogs.tableMetadata(r.Context(), profile, source.QueryRegion, requested.Table.Catalog, requested.Table.Database, requested.Table.Table, req.Workgroup)
			if err != nil {
				writeAccessLogError(w, accessLogTableError(err))
				return
			}
			columns := athenaColumnNames(meta.Columns)
			partitionKeys := athenaColumnNames(meta.PartitionKeys)
			profileName, ok := accesslog.DetectProfile(columns)
			if !ok {
				writeAccessLogError(w, fmt.Errorf("%w: cannot detect the schema profile of %s", accesslog.ErrInvalidTable, requested.Table.Table))
				return
			}
			if missing := accesslog.MissingColumns(profileName, columns); len(missing) > 0 {
				writeAccessLogError(w, fmt.Errorf("%w: missing columns %s", accesslog.ErrInvalidTable, strings.Join(missing, ", ")))
				return
			}
			sourceKeyColumn := accesslog.FindSourceKeyColumn(profileName, columns)
			groupKey := strings.Join([]string{source.QueryRegion, requested.Table.Catalog, requested.Table.Database, requested.Table.Table, string(profileName)}, "|")
			group := athenaGroups[groupKey]
			if group == nil {
				group = &accessLogAthenaGroup{
					region:          source.QueryRegion,
					catalog:         requested.Table.Catalog,
					database:        requested.Table.Database,
					table:           requested.Table.Table,
					profile:         profileName,
					columns:         columns,
					partitionKeys:   partitionKeys,
					partitionStyle:  accesslog.DetectPartitionStyle(partitionKeys),
					sourceKeyColumn: sourceKeyColumn,
				}
				athenaGroups[groupKey] = group
			}
			if group.sourceKeyColumn == "" && len(group.sources) > 0 {
				writeAccessLogError(w, fmt.Errorf("%w: table %s is shared but has no source key column", accesslog.ErrAttributionRequired, group.table))
				return
			}
			if group.sourceKeyColumn == "" && len(requested.Table.Table) > 0 && len(group.sources) == 0 {
				jobSource.DefaultSourceID = source.ID
			}
			group.sources = append(group.sources, accesslog.AthenaSource{ID: source.ID, KeyValues: source.SourceKeyValues})
			jobSource.QueryRegion = source.QueryRegion
			jobSource.State = accesslog.QueryStateQueued
			jobSources = append(jobSources, jobSource)
		case accesslog.EngineLogsInsights:
			if source.Destination.LogGroup == "" {
				writeAccessLogError(w, fmt.Errorf("%w: no log group for %s", accesslog.ErrSourceNotReady, source.Name))
				return
			}
			groupKey := source.QueryRegion + "|" + source.Destination.LogGroup
			group := insightsGroups[groupKey]
			if group == nil {
				group = &accessLogInsightsGroup{region: source.QueryRegion, logGroup: source.Destination.LogGroup}
				insightsGroups[groupKey] = group
			}
			group.sources = append(group.sources, accesslog.AthenaSource{ID: source.ID, KeyValues: source.SourceKeyValues})
			jobSource.QueryRegion = source.QueryRegion
			jobSource.LogGroup = source.Destination.LogGroup
			jobSource.State = accesslog.QueryStateQueued
			jobSources = append(jobSources, jobSource)
		default:
			writeAccessLogError(w, fmt.Errorf("%w: %s", accesslog.ErrSourceNotReady, source.Name))
			return
		}
	}

	job := &accessLogJob{
		ID:        newAccessLogJobID(),
		Profile:   profile,
		Region:    region,
		View:      req.View,
		CreatedAt: time.Now(),
		Sources:   jobSources,
		Warnings:  discovered.Warnings,
	}
	if req.Workgroup != "" && len(athenaGroups) > 0 {
		for _, group := range athenaGroups {
			workgroup, err := s.accessLogs.workgroup(r.Context(), profile, group.region, req.Workgroup)
			if err != nil {
				job.Warnings = append(job.Warnings, fmt.Sprintf("read Athena workgroup %s: %v", req.Workgroup, err))
				continue
			}
			if workgroup.BytesScannedCutoffPerQuery == 0 {
				job.Warnings = append(job.Warnings, fmt.Sprintf("Athena workgroup %s has no bytes-scanned cutoff", req.Workgroup))
			}
		}
	}
	for _, expired := range s.accessLogJobs.cleanup() {
		s.stopAccessLogJob(context.Background(), expired)
	}
	if err := s.startAccessLogQueries(r.Context(), job, athenaGroups, insightsGroups, timeRange, req); err != nil {
		s.stopAccessLogJob(context.Background(), job)
		writeAccessLogError(w, err)
		return
	}
	s.accessLogJobs.create(job)
	writeJSON(w, AccessLogQueryStartResponse{JobID: job.ID})
}

func (s *Server) startAccessLogQueries(ctx context.Context, job *accessLogJob, athenaGroups map[string]*accessLogAthenaGroup, insightsGroups map[string]*accessLogInsightsGroup, timeRange accesslog.TimeRange, req AccessLogQueryRequest) error {
	jobSourceByID := make(map[string]*accessLogJobSource, len(job.Sources))
	for _, source := range job.Sources {
		jobSourceByID[source.SourceID] = source
	}
	for _, group := range athenaGroups {
		plan := accesslog.AthenaPlan{
			Database:        group.database,
			Table:           group.table,
			Profile:         group.profile,
			Columns:         group.columns,
			PartitionStyle:  group.partitionStyle,
			PartitionKeys:   group.partitionKeys,
			SourceKeyColumn: group.sourceKeyColumn,
			Sources:         group.sources,
			TimeRange:       timeRange,
			Filter:          req.Filter,
			Limit:           req.Limit,
		}
		var query accesslog.AthenaQuery
		var err error
		if job.View == "ips" {
			query, err = accesslog.BuildIPQuery(plan)
		} else {
			query, err = accesslog.BuildPairQuery(plan)
		}
		if err != nil {
			return err
		}
		if query.FullScan && timeRange.To.Sub(timeRange.From) > accessLogDefaultRange {
			return fmt.Errorf("%w: the table has no partition for a range longer than %s", accesslog.ErrRangeTooLong, accessLogDefaultRange)
		}
		exec, err := s.accessLogs.startAthena(ctx, job.Profile, group.region, awsinternal.StartAthenaQueryInput{
			SQL:                 query.SQL,
			Catalog:             group.catalog,
			Database:            group.database,
			Workgroup:           req.Workgroup,
			OutputLocation:      req.OutputLocation,
			ExecutionParameters: query.Parameters,
		})
		if err != nil {
			return err
		}
		for _, source := range group.sources {
			jobSource := jobSourceByID[source.ID]
			if jobSource == nil {
				continue
			}
			jobSource.Engine = accesslog.EngineAthena
			jobSource.ExecutionID = exec.ID
			jobSource.State = athenaQueryState(exec.State)
			jobSource.SourceKeyToID = query.SourceKeyToID
			if len(group.sources) == 1 {
				jobSource.DefaultSourceID = source.ID
			}
		}
	}
	for _, group := range insightsGroups {
		plan := accesslog.InsightsPlan{
			LogGroups: []string{group.logGroup},
			Sources:   group.sources,
			TimeRange: timeRange,
			Filter:    req.Filter,
			Limit:     req.Limit,
		}
		query, err := accesslog.BuildWAFInsightsQuery(plan)
		if err != nil {
			return err
		}
		id, err := s.accessLogs.startInsights(ctx, job.Profile, group.region, []string{group.logGroup}, query.Query, timeRange.From, timeRange.To, req.Limit)
		if err != nil {
			return err
		}
		for _, source := range group.sources {
			jobSource := jobSourceByID[source.ID]
			if jobSource == nil {
				continue
			}
			jobSource.Engine = accesslog.EngineLogsInsights
			jobSource.ExecutionID = id
			jobSource.State = accesslog.QueryStateQueued
			jobSource.SourceKeyToID = query.SourceKeyToID
			if len(group.sources) == 1 {
				jobSource.DefaultSourceID = source.ID
			}
		}
	}
	return nil
}

func (s *Server) handleAccessLogQueryGet(w http.ResponseWriter, r *http.Request) {
	job, expired, ok := s.getAccessLogJob(w, r)
	if !ok {
		return
	}
	if expired {
		s.stopAccessLogJob(r.Context(), job)
		writeError(w, http.StatusGone, "ACCESS_LOGS_JOB_EXPIRED", "the access log query job has expired")
		return
	}
	s.refreshAccessLogJob(r.Context(), job)
	writeJSON(w, s.accessLogJobResponse(job))
}

func (s *Server) handleAccessLogQueryResults(w http.ResponseWriter, r *http.Request) {
	job, expired, ok := s.getAccessLogJob(w, r)
	if !ok {
		return
	}
	if expired {
		s.stopAccessLogJob(r.Context(), job)
		writeError(w, http.StatusGone, "ACCESS_LOGS_JOB_EXPIRED", "the access log query job has expired")
		return
	}
	s.refreshAccessLogJob(r.Context(), job)
	job.mu.Lock()
	defer job.mu.Unlock()
	state := accessLogJobState(job)
	if state == accesslog.QueryStateQueued || state == accesslog.QueryStateRunning {
		writeError(w, http.StatusConflict, "ACCESS_LOGS_QUERY_RUNNING", "the access log query is still running")
		return
	}
	sources := make([]accesslog.Source, 0, len(job.Sources))
	results := make([]accesslog.SourceResult, 0, len(job.Sources))
	for _, source := range job.Sources {
		sources = append(sources, accesslog.Source{ID: source.SourceID, Type: source.Type, Name: source.Name, OriginOf: source.OriginOf})
		result := accesslog.SourceResult{
			SourceID:       source.SourceID,
			Rows:           source.Rows,
			IPs:            source.IPs,
			BytesScanned:   source.BytesScanned,
			RecordsScanned: source.RecordsScanned,
		}
		if source.Err != "" {
			result.Err = errors.New(source.Err)
		}
		results = append(results, result)
	}
	merged := accesslog.Merge(results, sources)
	merged.Warnings = dedupeStrings(append(merged.Warnings, job.Warnings...))
	writeJSON(w, AccessLogResultsResponse{
		Rows:     merged.Rows,
		IPs:      merged.IPs,
		Summary:  merged.Summary,
		Warnings: merged.Warnings,
		Sources:  accessLogSourceStatuses(job),
	})
}

func (s *Server) handleAccessLogQueryCancel(w http.ResponseWriter, r *http.Request) {
	job, expired, ok := s.getAccessLogJob(w, r)
	if !ok {
		return
	}
	if expired {
		writeError(w, http.StatusGone, "ACCESS_LOGS_JOB_EXPIRED", "the access log query job has expired")
		return
	}
	s.stopAccessLogJob(r.Context(), job)
	writeJSON(w, s.accessLogJobResponse(job))
}

func (s *Server) getAccessLogJob(w http.ResponseWriter, r *http.Request) (*accessLogJob, bool, bool) {
	job, expired, ok := s.accessLogJobs.get(r.PathValue("job"))
	if !ok && !expired {
		writeError(w, http.StatusNotFound, "ACCESS_LOGS_JOB_NOT_FOUND", "the access log query job was not found")
		return nil, false, false
	}
	return job, expired, true
}

func (s *Server) accessLogSources(ctx context.Context, profile, region string) (awsinternal.AccessLogSources, error) {
	entry, _, err := s.resourceCache.Load(cacheKey("access-logs-sources", profile, region), time.Hour, false, func() (any, error) {
		return s.accessLogs.discover(ctx, profile, region)
	})
	if err != nil {
		return awsinternal.AccessLogSources{}, err
	}
	value, ok := entry.Value.(awsinternal.AccessLogSources)
	if !ok {
		return awsinternal.AccessLogSources{}, fmt.Errorf("unexpected access log sources cache value")
	}
	return value, nil
}

func (s *Server) refreshAccessLogJob(ctx context.Context, job *accessLogJob) {
	job.mu.Lock()
	defer job.mu.Unlock()
	for _, source := range job.Sources {
		if source.State == accesslog.QueryStateFailed || source.State == accesslog.QueryStateCancelled || source.State == accesslog.QueryStatePartial {
			continue
		}
		if source.State == accesslog.QueryStateSucceeded && source.Loaded {
			continue
		}
		switch source.Engine {
		case accesslog.EngineAthena:
			exec, err := s.accessLogs.getAthena(ctx, job.Profile, source.QueryRegion, source.ExecutionID)
			if err != nil {
				source.State = accesslog.QueryStateFailed
				source.Err = err.Error()
				continue
			}
			source.State = athenaQueryState(exec.State)
			source.BytesScanned = exec.BytesScanned
			if source.State == accesslog.QueryStateSucceeded && !source.Loaded {
				page, err := s.accessLogs.resultsAthena(ctx, job.Profile, source.QueryRegion, source.ExecutionID, 1000)
				if err != nil {
					source.State = accesslog.QueryStateFailed
					source.Err = err.Error()
					continue
				}
				pairs, ips, err := accesslog.ParseAthenaRows(athenaResultColumnNames(page.Columns), page.Rows, job.View, source.SourceKeyToID, source.DefaultSourceID)
				if err != nil {
					source.State = accesslog.QueryStateFailed
					source.Err = err.Error()
					continue
				}
				source.Rows = pairs
				source.IPs = ips
				source.Loaded = true
			}
		case accesslog.EngineLogsInsights:
			result, err := s.accessLogs.getInsights(ctx, job.Profile, source.QueryRegion, source.ExecutionID)
			if err != nil {
				source.State = accesslog.QueryStateFailed
				source.Err = err.Error()
				continue
			}
			source.State = insightsQueryState(result.Status)
			source.BytesScanned = result.BytesScanned
			source.RecordsScanned = result.RecordsScanned
			source.RecordsMatched = result.RecordsMatched
			if source.State == accesslog.QueryStateSucceeded && !source.Loaded {
				source.Rows = accesslog.ParseInsightsRows(result.Results, source.SourceKeyToID, source.DefaultSourceID)
				source.Loaded = true
			}
		default:
			source.State = accesslog.QueryStateFailed
			source.Err = "unsupported query engine"
		}
	}
}

func (s *Server) stopAccessLogJob(ctx context.Context, job *accessLogJob) {
	job.mu.Lock()
	defer job.mu.Unlock()
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, source := range job.Sources {
		if source.ExecutionID == "" {
			continue
		}
		if source.State != accesslog.QueryStateQueued && source.State != accesslog.QueryStateRunning {
			continue
		}
		switch source.Engine {
		case accesslog.EngineAthena:
			if err := s.accessLogs.stopAthena(stopCtx, job.Profile, source.QueryRegion, source.ExecutionID); err != nil {
				job.Warnings = append(job.Warnings, fmt.Sprintf("stop %s: %v", source.Name, err))
			}
		case accesslog.EngineLogsInsights:
			if err := s.accessLogs.stopInsights(stopCtx, job.Profile, source.QueryRegion, source.ExecutionID); err != nil {
				job.Warnings = append(job.Warnings, fmt.Sprintf("stop %s: %v", source.Name, err))
			}
		}
		source.State = accesslog.QueryStateCancelled
	}
}

func (s *Server) accessLogJobResponse(job *accessLogJob) AccessLogQueryResponse {
	job.mu.Lock()
	defer job.mu.Unlock()
	return AccessLogQueryResponse{
		JobID:    job.ID,
		State:    accessLogJobState(job),
		Sources:  accessLogSourceStatuses(job),
		Warnings: dedupeStrings(job.Warnings),
	}
}

func accessLogSourceStatuses(job *accessLogJob) []AccessLogQuerySourceStatus {
	out := make([]AccessLogQuerySourceStatus, 0, len(job.Sources))
	for _, source := range job.Sources {
		status := AccessLogQuerySourceStatus{
			SourceID:       source.SourceID,
			Name:           source.Name,
			Type:           source.Type,
			Engine:         source.Engine,
			State:          source.State,
			BytesScanned:   source.BytesScanned,
			RecordsScanned: source.RecordsScanned,
			RecordsMatched: source.RecordsMatched,
		}
		if source.Err != "" {
			status.Error = &AccessLogSourceError{Code: "ACCESS_LOGS_SOURCE_FAILED", Message: source.Err}
		}
		out = append(out, status)
	}
	return out
}

func accessLogJobState(job *accessLogJob) accesslog.QueryState {
	running := 0
	succeeded := 0
	failed := 0
	cancelled := 0
	for _, source := range job.Sources {
		switch source.State {
		case accesslog.QueryStateQueued, accesslog.QueryStateRunning:
			running++
		case accesslog.QueryStateSucceeded:
			succeeded++
		case accesslog.QueryStateFailed:
			failed++
		case accesslog.QueryStateCancelled:
			cancelled++
		}
	}
	switch {
	case running > 0:
		return accesslog.QueryStateRunning
	case succeeded == len(job.Sources):
		return accesslog.QueryStateSucceeded
	case succeeded > 0:
		return accesslog.QueryStatePartial
	case cancelled > 0 && failed == 0:
		return accesslog.QueryStateCancelled
	default:
		return accesslog.QueryStateFailed
	}
}

func newAccessLogJobSource(source accesslog.Source) *accessLogJobSource {
	return &accessLogJobSource{
		SourceID:    source.ID,
		Name:        source.Name,
		Type:        source.Type,
		Engine:      source.Engine,
		QueryRegion: source.QueryRegion,
		OriginOf:    source.OriginOf,
		State:       accesslog.QueryStateQueued,
	}
}

func findAccessLogSource(sources []accesslog.Source, id string) (accesslog.Source, bool) {
	for _, source := range sources {
		if source.ID == id {
			return source, true
		}
	}
	return accesslog.Source{}, false
}

func athenaColumnNames(columns []awsinternal.AthenaColumn) []string {
	out := make([]string, 0, len(columns))
	for _, column := range columns {
		out = append(out, column.Name)
	}
	return out
}

func athenaResultColumnNames(columns []awsinternal.AthenaResultColumn) []string {
	out := make([]string, 0, len(columns))
	for _, column := range columns {
		out = append(out, column.Name)
	}
	return out
}

func athenaQueryState(state string) accesslog.QueryState {
	switch strings.ToUpper(state) {
	case "SUCCEEDED":
		return accesslog.QueryStateSucceeded
	case "FAILED":
		return accesslog.QueryStateFailed
	case "CANCELLED":
		return accesslog.QueryStateCancelled
	case "QUEUED":
		return accesslog.QueryStateQueued
	default:
		return accesslog.QueryStateRunning
	}
}

func insightsQueryState(state string) accesslog.QueryState {
	switch state {
	case "Complete":
		return accesslog.QueryStateSucceeded
	case "Failed", "Timeout":
		return accesslog.QueryStateFailed
	case "Cancelled":
		return accesslog.QueryStateCancelled
	case "Scheduled":
		return accesslog.QueryStateQueued
	default:
		return accesslog.QueryStateRunning
	}
}
