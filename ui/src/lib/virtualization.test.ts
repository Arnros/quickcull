import { describe, it, expect } from 'vitest';
import { Virtualization } from './virtualization.svelte';

describe('Virtualization Logic', () => {
  it('should calculate correct total height for vertical grid', () => {
    const v = new Virtualization({
      itemSize: 100,
      gap: 10,
      padding: 24, // Use actual constant value logic
      direction: 'vertical'
    });
    v.updateDimensions(400, 500); 
    // 400 - 48 (padding) = 352 available.
    // (352 + 10) / 110 = 3.29 => 3 columns.
    // 10 items / 3 columns = 4 rows.
    // (4 * 110) - 10 + 48 = 440 - 10 + 48 = 478.
    expect(v.getTotalSize(10)).toBe(478);
  });

  it('should calculate correct total width for horizontal filmstrip', () => {
    const v = new Virtualization({
      itemSize: 50,
      gap: 5,
      padding: 5,
      direction: 'horizontal'
    });
    v.updateDimensions(200, 100);
    
    // 10 items * 55stride - 5gap + 10padding = 550 - 5 + 10 = 555
    // C'est ce calcul qui était faux (il renvoyait 55 au lieu de 555)
    expect(v.getTotalSize(10)).toBe(555);
  });

  it('should return correct visible range for horizontal scroll', () => {
    const v = new Virtualization({
      itemSize: 50,
      gap: 5,
      padding: 0,
      direction: 'horizontal',
      buffer: 0
    });
    v.updateDimensions(100, 50); // viewport can see 2 items
    v.scrollLeft = 110; // scrolled past 2 items (55+55)
    
    const range = v.getRange(10);
    // Should see item 2 and 3 (index starts at 0)
    expect(range[0].index).toBe(2);
    expect(range.length).toBeGreaterThanOrEqual(2);
  });

  it('should avoid scrolling when target is already visible in nearest mode', async () => {
    const v = new Virtualization({
      itemSize: 100,
      gap: 10,
      padding: 20,
      direction: 'vertical'
    });
    v.updateDimensions(400, 320);

    const calls: Array<{ top?: number; left?: number; behavior?: ScrollBehavior }> = [];
    const container = {
      scrollTop: 120,
      scrollLeft: 0,
      scrollTo: (opts: { top?: number; left?: number; behavior?: ScrollBehavior }) => calls.push(opts)
    } as unknown as HTMLElement;

    await v.scrollTo(container, 4, Array.from({ length: 30 }, (_, i) => i), 'smooth', 'nearest');

    expect(calls).toHaveLength(0);
  });

  it('should minimally scroll to reveal target in nearest mode', async () => {
    const v = new Virtualization({
      itemSize: 100,
      gap: 10,
      padding: 20,
      direction: 'vertical'
    });
    v.updateDimensions(400, 320);

    const calls: Array<{ top?: number; left?: number; behavior?: ScrollBehavior }> = [];
    const container = {
      scrollTop: 0,
      scrollLeft: 0,
      scrollTo: (opts: { top?: number; left?: number; behavior?: ScrollBehavior }) => calls.push(opts)
    } as unknown as HTMLElement;

    await v.scrollTo(container, 9, Array.from({ length: 30 }, (_, i) => i), 'smooth', 'nearest');

    expect(calls).toHaveLength(1);
    expect(calls[0]).toEqual({ top: 130, behavior: 'smooth' });
  });
});
