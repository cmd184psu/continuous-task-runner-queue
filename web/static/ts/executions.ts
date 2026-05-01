import { api, TaskExecution } from './api.js';

function statusBadge(status: string): HTMLElement {
  const span = document.createElement('span');
  const map: Record<string, string> = {
    success: 'badge-green',
    failed: 'badge-red',
    running: 'badge-blue',
    pending: 'badge-yellow',
  };
  span.className = 'badge ' + (map[status] ?? 'badge-muted');
  span.textContent = status;
  return span;
}

function renderExecRow(exec: TaskExecution, tbody: HTMLElement): void {
  const tr = document.createElement('tr');

  const tdId = document.createElement('td');
  const link = document.createElement('a');
  link.href = '#output/' + exec.id;
  link.textContent = String(exec.id);
  link.style.color = 'var(--text-accent)';
  tdId.appendChild(link);
  tr.appendChild(tdId);

  const tdTask = document.createElement('td');
  tdTask.textContent = exec.task_name ?? String(exec.task_id);
  tr.appendChild(tdTask);

  const tdStatus = document.createElement('td');
  tdStatus.appendChild(statusBadge(exec.status));
  tr.appendChild(tdStatus);

  const tdDuration = document.createElement('td');
  tdDuration.textContent = exec.duration_ms != null ? exec.duration_ms + 'ms' : '-';
  tr.appendChild(tdDuration);

  const tdDelay = document.createElement('td');
  tdDelay.textContent = exec.schedule_delay_ms != null ? exec.schedule_delay_ms + 'ms' : '-';
  tr.appendChild(tdDelay);

  const tdStarted = document.createElement('td');
  tdStarted.textContent = exec.started_at ? new Date(exec.started_at).toLocaleString() : '-';
  tr.appendChild(tdStarted);

  const tdError = document.createElement('td');
  if (exec.error_message) {
    const code = document.createElement('code');
    code.textContent = exec.error_message.slice(0, 80);
    code.title = exec.error_message;
    tdError.appendChild(code);
  } else {
    tdError.textContent = '-';
  }
  tr.appendChild(tdError);

  tbody.appendChild(tr);
}

export function renderExecutions(container: HTMLElement, taskFilter?: string): void {
  container.textContent = '';

  const toolbar = document.createElement('div');
  toolbar.className = 'toolbar';

  const h1 = document.createElement('h1');
  h1.textContent = taskFilter ? 'Executions — ' + taskFilter : 'Executions';
  toolbar.appendChild(h1);

  const spacer = document.createElement('div');
  spacer.className = 'toolbar-spacer';
  toolbar.appendChild(spacer);

  const btnRefresh = document.createElement('button');
  btnRefresh.className = 'btn btn-secondary';
  btnRefresh.textContent = 'Refresh';
  btnRefresh.addEventListener('click', () => renderExecutions(container, taskFilter));
  toolbar.appendChild(btnRefresh);

  container.appendChild(toolbar);

  api.listExecutions(taskFilter, 50).then((execs) => {
    if (execs.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'empty-state';
      empty.textContent = 'No executions yet.';
      container.appendChild(empty);
      return;
    }

    const card = document.createElement('div');
    card.className = 'card';

    const table = document.createElement('table');
    const thead = document.createElement('thead');
    const headerRow = document.createElement('tr');
    ['ID', 'Task', 'Status', 'Duration', 'Delay', 'Started', 'Error'].forEach((h) => {
      const th = document.createElement('th');
      th.textContent = h;
      headerRow.appendChild(th);
    });
    thead.appendChild(headerRow);
    table.appendChild(thead);

    const tbody = document.createElement('tbody');
    execs.forEach((e) => renderExecRow(e, tbody));
    table.appendChild(tbody);
    card.appendChild(table);
    container.appendChild(card);
  }).catch((e: Error) => {
    const err = document.createElement('div');
    err.className = 'error-banner';
    err.textContent = e.message;
    container.appendChild(err);
  });
}
