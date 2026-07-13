import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { GcpProjectSelect } from './GcpProjectSelect';
import type { GcpProject } from '../types/gcp';

const PROJECTS: GcpProject[] = [
  {
    id: 'prod-alpha',
    name: 'Prod Alpha',
    projectNumber: '111111111111',
    state: 'ACTIVE',
    createTime: '2024-01-01T00:00:00Z',
  },
  {
    id: 'stg-beta',
    name: 'Stg Beta',
    projectNumber: '222222222222',
    state: 'ACTIVE',
    createTime: '2024-01-02T00:00:00Z',
  },
  {
    id: 'dev-gamma',
    name: 'Dev Gamma',
    projectNumber: '333333333333',
    state: 'ACTIVE',
    createTime: '2024-01-03T00:00:00Z',
  },
];

describe('GcpProjectSelect', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('トリガーに選択中プロジェクトの名前と id を表示する', () => {
    render(
      <GcpProjectSelect project="prod-alpha" projects={PROJECTS} onProjectChange={() => {}} />,
    );
    expect(screen.getByText('Prod Alpha')).toBeInTheDocument();
    expect(screen.getByText('prod-alpha')).toBeInTheDocument();
  });

  it('トリガークリックで検索ボックスと候補一覧が開く', () => {
    render(
      <GcpProjectSelect project="prod-alpha" projects={PROJECTS} onProjectChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /Prod Alpha/ }));
    expect(screen.getByPlaceholderText('filter by name, project id…')).toBeInTheDocument();
    expect(screen.getAllByRole('option')).toHaveLength(3);
  });

  it('name で絞り込める', () => {
    render(
      <GcpProjectSelect project="prod-alpha" projects={PROJECTS} onProjectChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /Prod Alpha/ }));
    fireEvent.change(screen.getByPlaceholderText('filter by name, project id…'), {
      target: { value: 'Beta' },
    });
    const options = screen.getAllByRole('option');
    expect(options).toHaveLength(1);
    expect(options[0]).toHaveTextContent('Stg Beta');
  });

  it('project id で絞り込める', () => {
    render(
      <GcpProjectSelect project="prod-alpha" projects={PROJECTS} onProjectChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /Prod Alpha/ }));
    fireEvent.change(screen.getByPlaceholderText('filter by name, project id…'), {
      target: { value: 'dev-gamma' },
    });
    const options = screen.getAllByRole('option');
    expect(options).toHaveLength(1);
    expect(options[0]).toHaveTextContent('Dev Gamma');
  });

  it('候補をクリックすると onProjectChange が呼ばれメニューが閉じる', () => {
    const onProjectChange = vi.fn();
    render(
      <GcpProjectSelect
        project="prod-alpha"
        projects={PROJECTS}
        onProjectChange={onProjectChange}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /Prod Alpha/ }));
    fireEvent.click(screen.getByText('Stg Beta'));
    expect(onProjectChange).toHaveBeenCalledWith('stg-beta');
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });

  it('Enter キーで active な候補を確定する', () => {
    const onProjectChange = vi.fn();
    render(
      <GcpProjectSelect
        project="prod-alpha"
        projects={PROJECTS}
        onProjectChange={onProjectChange}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /Prod Alpha/ }));
    const input = screen.getByPlaceholderText('filter by name, project id…');
    fireEvent.keyDown(input, { key: 'ArrowDown' });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onProjectChange).toHaveBeenCalledWith('stg-beta');
  });

  it('Escape キーでメニューを閉じる', () => {
    render(
      <GcpProjectSelect project="prod-alpha" projects={PROJECTS} onProjectChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /Prod Alpha/ }));
    const input = screen.getByPlaceholderText('filter by name, project id…');
    fireEvent.keyDown(input, { key: 'Escape' });
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });

  it('外側クリックでメニューを閉じる', async () => {
    render(
      <div>
        <div data-testid="outside" />
        <GcpProjectSelect project="prod-alpha" projects={PROJECTS} onProjectChange={() => {}} />
      </div>,
    );
    fireEvent.click(screen.getByRole('button', { name: /Prod Alpha/ }));
    expect(screen.getByRole('listbox')).toBeInTheDocument();
    fireEvent.pointerDown(screen.getByTestId('outside'));
    await waitFor(() => {
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    });
  });

  it('一致する候補がない場合は空状態を表示する', () => {
    render(
      <GcpProjectSelect project="prod-alpha" projects={PROJECTS} onProjectChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /Prod Alpha/ }));
    fireEvent.change(screen.getByPlaceholderText('filter by name, project id…'), {
      target: { value: 'no-such' },
    });
    expect(screen.getByText('No projects match')).toBeInTheDocument();
  });
});
