import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import { TweaksPanel } from './TweaksPanel';
import { resetTweaksForTest } from '../hooks/useTweaks';
import i18n from '../i18n';

describe('TweaksPanel', () => {
  beforeEach(() => {
    localStorage.clear();
    resetTweaksForTest();
    void i18n.changeLanguage('ja');
  });

  it('Language 行から日本語/英語を切り替えられる (issue 0050)', () => {
    render(<TweaksPanel />);

    fireEvent.click(screen.getByRole('button', { name: 'English' }));
    expect(i18n.language).toBe('en');

    fireEvent.click(screen.getByRole('button', { name: '日本語' }));
    expect(i18n.language).toBe('ja');
  });

  it('現在の言語ボタンに active クラスが付く', () => {
    render(<TweaksPanel />);

    const jaButton = screen.getByRole('button', { name: '日本語' });
    const enButton = screen.getByRole('button', { name: 'English' });
    expect(jaButton.className).toContain('active');
    expect(enButton.className).not.toContain('active');

    fireEvent.click(enButton);
    expect(enButton.className).toContain('active');
    expect(jaButton.className).not.toContain('active');
  });

  it('Layout 行は Theme の直後にあり、Workbench を選ぶと data-layout が切り替わる (issue 0210)', () => {
    const { container } = render(<TweaksPanel />);

    const labels = Array.from(container.querySelectorAll('.trow .lbl')).map((el) => el.textContent);
    expect(labels.slice(0, 2)).toEqual(['Theme', 'Layout']);

    const standard = screen.getByRole('button', { name: 'Standard' });
    const workbench = screen.getByRole('button', { name: 'Workbench' });
    expect(standard.className).toContain('active');
    expect(document.documentElement.getAttribute('data-layout')).toBe('standard');

    fireEvent.click(workbench);
    expect(workbench.className).toContain('active');
    expect(standard.className).not.toContain('active');
    expect(document.documentElement.getAttribute('data-layout')).toBe('workbench');
  });
});
