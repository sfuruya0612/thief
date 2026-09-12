import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { StatTile } from './StatTile';

describe('StatTile', () => {
  it('タイトルと値を表示する', () => {
    render(<StatTile title="Load" value={12.3456} />);
    expect(screen.getByText('Load')).toBeInTheDocument();
    expect(screen.getByText('12.35')).toBeInTheDocument();
  });

  it('単位を値に添える', () => {
    render(<StatTile title="Hosts" value={7} unit="hosts" />);
    expect(screen.getByText('hosts')).toBeInTheDocument();
  });

  it('値が無いときは 0 と区別できるダッシュを表示する', () => {
    render(<StatTile title="Load" value={null} />);
    expect(screen.getByText('—')).toBeInTheDocument();
  });

  it('ホバーで丸めていない値を読める', () => {
    render(<StatTile title="Load" value={12.3456} unit="req/s" />);
    expect(screen.getByTitle('Load: 12.3456 req/s')).toBeInTheDocument();
  });
});
