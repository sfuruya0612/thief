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
});
