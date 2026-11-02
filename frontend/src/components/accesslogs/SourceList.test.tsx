import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { SourceList } from './SourceList';
import type { AccessLogSource } from '../../types/accesslogs';

function source(overrides: Partial<AccessLogSource>): AccessLogSource {
  return {
    id: 'source-1',
    type: 'alb',
    name: 'prod-alb',
    region: 'ap-northeast-1',
    queryRegion: 'ap-northeast-1',
    global: false,
    aliases: [],
    engine: 'athena',
    destination: { type: 's3', bucket: 'logs', prefix: 'alb' },
    status: {
      loggingEnabled: true,
      destinationType: 's3',
      destinationReadable: true,
      tableConfigured: false,
      tableValid: false,
      attribution: true,
      sharedBy: [],
      issues: [],
    },
    sourceKeyValues: [],
    ...overrides,
  };
}

describe('SourceList', () => {
  it('shows ready and disabled sources', () => {
    const onToggle = vi.fn();
    const sources = [
      source({
        id: 'ready',
        name: 'ready-alb',
        status: { ...source({}).status, sharedBy: ['other-alb'] },
      }),
      source({
        id: 'disabled',
        name: 'disabled-alb',
        engine: '',
        destination: { type: 'none' },
        status: {
          ...source({}).status,
          loggingEnabled: false,
          destinationType: 'none',
          issues: ['Access logging is not enabled'],
        },
      }),
    ];
    render(
      <SourceList
        sources={sources}
        selected={new Set(['ready'])}
        onToggle={onToggle}
        onSelectAll={() => undefined}
        onRecheck={() => undefined}
      />,
    );

    expect(screen.getByText('ready-alb')).toBeInTheDocument();
    expect(screen.getByText('disabled-alb')).toBeInTheDocument();
    expect(screen.getByText('Logging disabled')).toBeInTheDocument();
    expect(screen.getByText('Shared')).toBeInTheDocument();
    const checkboxes = screen.getAllByRole('checkbox');
    expect(checkboxes[0]).toBeChecked();
    expect(checkboxes[1]).toBeDisabled();
    fireEvent.click(checkboxes[0]);
    expect(onToggle).toHaveBeenCalledWith('ready');
  });
});
