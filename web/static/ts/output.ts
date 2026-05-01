import { hasToken } from './api.js';

export function renderOutput(container: HTMLElement, execID: string): void {
  container.textContent = '';

  const h1 = document.createElement('h1');
  h1.textContent = 'Output — execution ' + execID;
  container.appendChild(h1);

  const statusDiv = document.createElement('div');
  statusDiv.style.marginBottom = '12px';
  container.appendChild(statusDiv);

  const pre = document.createElement('pre');
  pre.className = 'output-terminal';
  container.appendChild(pre);

  if (!hasToken()) {
    const err = document.createElement('div');
    err.className = 'error-banner';
    err.textContent = 'Not authenticated.';
    container.appendChild(err);
    return;
  }

  const token = sessionStorage.getItem('ctrq-token') ?? '';
  const controller = new AbortController();

  // Use fetch + ReadableStream instead of EventSource — EventSource cannot send auth headers.
  fetch('/api/executions/' + execID + '/output', {
    headers: { 'Authorization': 'Bearer ' + token },
    signal: controller.signal,
  }).then(async (resp) => {
    if (!resp.ok) {
      const msg = document.createElement('div');
      msg.className = 'error-banner';
      msg.textContent = 'Error ' + resp.status + ': ' + resp.statusText;
      container.appendChild(msg);
      return;
    }
    if (!resp.body) return;

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() ?? '';

      let eventType = '';
      for (const line of lines) {
        if (line.startsWith('event: ')) {
          eventType = line.slice(7).trim();
        } else if (line.startsWith('data: ')) {
          const data = line.slice(6);
          if (eventType === 'output') {
            try {
              const parsed = JSON.parse(data) as { stream: string; line: string };
              const span = document.createElement('span');
              span.className = 'stream-' + parsed.stream;
              span.textContent = parsed.line + '\n';
              pre.appendChild(span);
              pre.scrollTop = pre.scrollHeight;
            } catch { /* ignore malformed */ }
          } else if (eventType === 'status') {
            statusDiv.textContent = 'Status: ' + data;
          } else if (eventType === 'done') {
            const done = document.createElement('div');
            done.style.color = 'var(--text-muted)';
            done.style.marginTop = '8px';
            done.style.fontSize = '12px';
            done.textContent = '— execution complete —';
            container.appendChild(done);
            return;
          }
          eventType = '';
        }
      }
    }
  }).catch((err: Error) => {
    if (err.name === 'AbortError') return;
    const msg = document.createElement('div');
    msg.className = 'error-banner';
    msg.textContent = err.message;
    container.appendChild(msg);
  });

  // Abort stream when user navigates away
  window.addEventListener('hashchange', () => controller.abort(), { once: true });
}
