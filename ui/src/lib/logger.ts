async function sendToBackend(level: string, message: string, context?: any) {
  try {
    if ((window as any).go?.review?.App?.Log) {
      await (window as any).go.review.App.Log(level, message, context || {});
    }
  } catch (e) {
    // Fallback to console if backend is unavailable
    console.error("Failed to send log to backend", e);
  }
}

export const logger = {
  debug(msg: string, ctx?: any) {
    console.debug(msg, ctx);
    sendToBackend('debug', msg, ctx);
  },
  info(msg: string, ctx?: any) {
    console.info(msg, ctx);
    sendToBackend('info', msg, ctx);
  },
  warn(msg: string, ctx?: any) {
    console.warn(msg, ctx);
    sendToBackend('warn', msg, ctx);
  },
  error(msg: string, ctx?: any) {
    console.error(msg, ctx);
    sendToBackend('error', msg, ctx);
  }
};

// Global error handler
if (typeof window !== 'undefined') {
  window.addEventListener('error', (event) => {
    logger.error('Uncaught JS Error', {
      message: event.message,
      filename: event.filename,
      lineno: event.lineno,
      colno: event.colno,
      stack: event.error?.stack
    });
  });

  window.addEventListener('unhandledrejection', (event) => {
    logger.error('Unhandled Promise Rejection', {
      reason: event.reason
    });
  });
}
