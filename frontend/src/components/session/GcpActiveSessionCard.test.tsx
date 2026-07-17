import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import type { GcpProject } from '../../types/gcp';
import { GcpActiveSessionCard } from './GcpActiveSessionCard';

afterEach(cleanup);

const projects: GcpProject[] = [
  { id: 'example-prod', name: 'Example Prod', projectNumber: '1', state: 'ACTIVE', createTime: '' },
];

describe('GcpActiveSessionCard', () => {
  it('プロジェクト表示名と ID と ADC ラベルを表示する', () => {
    render(<GcpActiveSessionCard project="example-prod" projects={projects} />);
    expect(screen.getByText('Example Prod')).toBeInTheDocument();
    expect(screen.getByText('example-prod')).toBeInTheDocument();
    expect(screen.getByText('ADC')).toBeInTheDocument();
  });

  it('環境色ドットがプロジェクト ID サフィックスに追随する', () => {
    const { container } = render(<GcpActiveSessionCard project="example-prod" projects={projects} />);
    expect(container.querySelector('.session-tab-dot')).toHaveClass('env-prod');
  });

  it('一覧に無いプロジェクトは ID をそのまま表示する', () => {
    render(<GcpActiveSessionCard project="ghost-proj" projects={[]} />);
    expect(screen.getAllByText('ghost-proj').length).toBeGreaterThan(0);
  });
});
