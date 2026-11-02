import { describe, expect, it } from 'vitest';
import {
  accessLogDestinationKey,
  accessLogIPFromRaw,
  accessLogPairFromRaw,
  accessLogResultsFromRaw,
  accessLogSourceFromRaw,
  accessLogSummaryFromRaw,
} from './normalizeAccessLogs';
import type { AccessLogSourceRaw } from '../types/accesslogs';

const sourceRaw: AccessLogSourceRaw = {
  id: 'source-1',
  type: 'alb',
  name: 'prod-alb',
  region: 'ap-northeast-1',
  query_region: 'ap-northeast-1',
  engine: 'athena',
  destination: { type: 's3', bucket: 'logs', prefix: 'alb' },
  status: {
    logging_enabled: true,
    destination_type: 's3',
    destination_readable: true,
    table_configured: false,
    table_valid: false,
    attribution: true,
    shared_by: ['prod-alb-2'],
    issues: ['shared'],
  },
  source_key_values: ['app/prod-alb/abc'],
  suggested_table: 'alb_access_logs',
};

describe('normalizeAccessLogs', () => {
  it('converts a source and destination key', () => {
    const source = accessLogSourceFromRaw(sourceRaw);
    expect(source.name).toBe('prod-alb');
    expect(source.queryRegion).toBe('ap-northeast-1');
    expect(source.status.sharedBy).toEqual(['prod-alb-2']);
    expect(source.sourceKeyValues).toEqual(['app/prod-alb/abc']);
    expect(accessLogDestinationKey(source)).toBe('s3://logs/alb');
  });

  it('converts a pair row with source labels', () => {
    const row = accessLogPairFromRaw(
      {
        client_ip: '203.0.113.9',
        uri_path: '/admin',
        request_count: 10,
        blocked_count: 3,
        challenged_count: 1,
        count_matched: 0,
        client_error_count: 4,
        server_error_count: 0,
        sources: ['source-1'],
        flags: ['blocked', 'sensitive_path'],
        has_query: true,
        sample_query: 'q=1',
      },
      { 'source-1': 'prod-alb' },
    );
    expect(row.id).toBe('203.0.113.9|/admin');
    expect(row.sourceLabels).toEqual(['prod-alb']);
    expect(row.blockedCount).toBe(3);
    expect(row.hasQuery).toBe(true);
    expect(row.sampleQuery).toBe('q=1');
    expect(row.state).toBe('blocked');
  });

  it('converts an IP row', () => {
    const row = accessLogIPFromRaw(
      {
        client_ip: '198.51.100.7',
        request_count: 20,
        blocked_count: 1,
        challenged_count: 0,
        count_matched: 0,
        client_error_count: 2,
        server_error_count: 0,
        distinct_uris: 7,
        sources: [],
      },
      {},
    );
    expect(row.distinctUris).toBe(7);
    expect(row.sourceLabels).toEqual([]);
  });

  it('converts a summary and results', () => {
    const summary = accessLogSummaryFromRaw({
      source_count: 1,
      record_count: 10,
      blocked_count: 3,
      challenged_count: 1,
      client_error_count: 4,
      server_error_count: 0,
      distinct_ips: 1,
      distinct_uris: 1,
      by_sources: [
        {
          source_id: 'source-1',
          records: 10,
          blocked: 3,
          challenged: 1,
          client_errors: 4,
          server_errors: 0,
          bytes_scanned: 1024,
          records_scanned: 10,
        },
      ],
      duplicate_warning: false,
    });
    expect(summary.bySources[0].bytesScanned).toBe(1024);
    const results = accessLogResultsFromRaw(
      {
        rows: [
          {
            client_ip: '203.0.113.9',
            uri_path: '/admin',
            request_count: 10,
            blocked_count: 3,
            challenged_count: 1,
            count_matched: 0,
            client_error_count: 4,
            server_error_count: 0,
          },
        ],
        ips: [],
        summary: {
          source_count: 1,
          record_count: 10,
          blocked_count: 3,
          challenged_count: 1,
          client_error_count: 4,
          server_error_count: 0,
          distinct_ips: 1,
          distinct_uris: 1,
          duplicate_warning: false,
        },
        sources: [],
      },
      {},
    );
    expect(results.rows).toHaveLength(1);
    expect(results.summary.recordCount).toBe(10);
  });
});
