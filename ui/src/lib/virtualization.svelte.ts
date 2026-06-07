import { tick } from "svelte";

interface VirtualizationOptions {
  itemSize: number;
  gap: number;
  padding: number;
  buffer?: number;
  direction?: 'vertical' | 'horizontal';
}

export class Virtualization {
  private options: VirtualizationOptions;
  
  scrollTop = $state(0);
  scrollLeft = $state(0);
  containerWidth = $state(0);
  containerHeight = $state(0);

  constructor(options: VirtualizationOptions) {
    this.options = { buffer: 2, direction: 'vertical', ...options };
  }

  updateDimensions(width: number, height: number) {
    this.containerWidth = width;
    this.containerHeight = height;
  }

  handleScroll(e: UIEvent) {
    const target = e.target as HTMLElement;
    this.scrollTop = target.scrollTop;
    this.scrollLeft = target.scrollLeft;
  }

  get columns() {
    if (this.options.direction === 'horizontal') return 1;
    return Math.max(
      1,
      Math.floor((this.containerWidth - this.options.padding * 2 + this.options.gap) / (this.options.itemSize + this.options.gap))
    );
  }

  getRange(itemCount: number) {
    if (this.containerHeight === 0 && this.containerWidth === 0) return [];

    const { itemSize, gap, padding, buffer, direction } = this.options;
    const isVertical = direction === 'vertical';
    const scrollPos = isVertical ? this.scrollTop : this.scrollLeft;
    const containerSize = isVertical ? this.containerHeight : this.containerWidth;
    
    // If container size is unknown but items exist, show a small initial range to prevent blank state
    const effectiveSize = containerSize || 1000;

    const stride = itemSize + gap;
    const columns = this.columns;
    const rows = isVertical ? Math.ceil(itemCount / columns) : itemCount;

    const startRow = Math.max(0, Math.floor((scrollPos - padding) / stride) - (buffer || 0));
    const endRow = Math.min(rows, Math.ceil((scrollPos + effectiveSize - padding) / stride) + (buffer || 0));

    const visible = [];
    if (isVertical) {
      for (let r = startRow; r < endRow; r++) {
        for (let c = 0; c < columns; c++) {
          const index = r * columns + c;
          if (index < itemCount) {
            visible.push({
              index,
              x: c * stride + padding,
              y: r * stride + padding
            });
          }
        }
      }
    } else {
      // Horizontal (Filmstrip)
      for (let i = startRow; i < endRow; i++) {
        if (i < itemCount) {
          visible.push({
            index: i,
            x: i * stride + padding,
            y: padding
          });
        }
      }
    }
    return visible;
  }

  getTotalSize(itemCount: number) {
    const { itemSize, gap, padding, direction } = this.options;
    const isVertical = direction === 'vertical';
    const columns = this.columns;
    const rows = isVertical ? Math.ceil(itemCount / columns) : itemCount;
    
    return Math.max(0, rows * (itemSize + gap) - gap + padding * 2);
  }

  async scrollTo(
    container: HTMLElement,
    index: number,
    items: (number | null)[],
    behavior: ScrollBehavior = 'auto',
    mode: 'center' | 'nearest' = 'center'
  ) {
    if (!container) return;
    await tick();
    
    const { itemSize, gap, padding, direction } = this.options;
    const isVertical = direction === 'vertical';
    const columns = this.columns;
    const idx = items.indexOf(index);
    if (idx === -1) return;

    if (isVertical) {
      const row = Math.floor(idx / columns);
      const itemTop = row * (itemSize + gap) + padding;
      const itemBottom = itemTop + itemSize;
      const viewportTop = container.scrollTop;
      const viewportBottom = viewportTop + this.containerHeight;
      let targetY = viewportTop;
      if (mode === 'center') {
        targetY = Math.max(0, itemTop - (this.containerHeight - itemSize) / 2);
      } else if (itemTop < viewportTop) {
        targetY = Math.max(0, itemTop);
      } else if (itemBottom > viewportBottom) {
        targetY = Math.max(0, itemBottom - this.containerHeight);
      } else {
        return;
      }
      container.scrollTo({ top: targetY, behavior });
    } else {
      const itemLeft = idx * (itemSize + gap) + padding;
      const itemRight = itemLeft + itemSize;
      const viewportLeft = container.scrollLeft;
      const viewportRight = viewportLeft + this.containerWidth;
      let targetX = viewportLeft;
      if (mode === 'center') {
        targetX = Math.max(0, itemLeft - (this.containerWidth - itemSize) / 2);
      } else if (itemLeft < viewportLeft) {
        targetX = Math.max(0, itemLeft);
      } else if (itemRight > viewportRight) {
        targetX = Math.max(0, itemRight - this.containerWidth);
      } else {
        return;
      }
      container.scrollTo({ left: targetX, behavior });
    }
  }
}
