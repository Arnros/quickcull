class ToastService {
  toast = $state<{ msg: string; type: string } | null>(null);
  private timer: any = null;

  show(msg: string, type: string = 'info') {
    this.toast = { msg, type };
    if (this.timer) clearTimeout(this.timer);
    this.timer = setTimeout(() => {
      this.toast = null;
    }, 3000);
  }

  success(msg: string) { this.show(msg, 'success'); }
  error(msg: string) { this.show(msg, 'danger'); }
  info(msg: string) { this.show(msg, 'info'); }
  star(msg: string) { this.show(msg, 'star'); }
}

export const toastService = new ToastService();
