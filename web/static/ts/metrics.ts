import { api, MetricSummary } from './api.js';

function tile(label: string, value: string): HTMLElement {
  const div = document.createElement('div');
  div.className = 'metric-tile';

  const lbl = document.createElement('div');
  lbl.className = 'metric-label';
  lbl.textContent = label;

  const val = document.createElement('div');
  val.className = 'metric-value';
  val.textContent = value;

  div.appendChild(lbl);
  div.appendChild(val);
  return div;
}

function renderMetricCard(s: MetricSummary, container: HTMLElement): void {
  const card = document.createElement('div');
  card.className = 'card';

  const header = document.createElement('div');
  header.className = 'card-header';

  const title = document.createElement('h2');
  title.textContent = s.task_name;
  header.appendChild(title);

  const groupBadge = document.createElement('span');
  groupBadge.className = 'badge badge-muted';
  groupBadge.textContent = s.group_name;
  header.appendChild(groupBadge);

  card.appendChild(header);

  const grid = document.createElement('div');
  grid.className = 'metrics-grid';

  const total = s.success_count + s.failed_count;
  const successPct = total > 0 ? Math.round((s.success_count / total) * 100) : 0;

  grid.appendChild(tile('Success', String(s.success_count)));
  grid.appendChild(tile('Failed', String(s.failed_count)));
  grid.appendChild(tile('Success rate', successPct + '%'));
  grid.appendChild(tile('Avg duration', s.avg_duration_ms != null ? Math.round(s.avg_duration_ms) + 'ms' : '—'));
  grid.appendChild(tile('Min duration', s.min_duration_ms != null ? s.min_duration_ms + 'ms' : '—'));
  grid.appendChild(tile('Max duration', s.max_duration_ms != null ? s.max_duration_ms + 'ms' : '—'));
  grid.appendChild(tile('Avg delay', s.avg_schedule_delay_ms != null ? Math.round(s.avg_schedule_delay_ms) + 'ms' : '—'));
  grid.appendChild(tile('Last run', s.last_execution ? new Date(s.last_execution).toLocaleString() : '—'));

  card.appendChild(grid);
  container.appendChild(card);
}

export function renderMetrics(container: HTMLElement, groupFilter?: string): void {
  container.textContent = '';

  const toolbar = document.createElement('div');
  toolbar.className = 'toolbar';

  const h1 = document.createElement('h1');
  h1.textContent = 'Metrics';
  toolbar.appendChild(h1);

  const spacer = document.createElement('div');
  spacer.className = 'toolbar-spacer';
  toolbar.appendChild(spacer);

  const btnRefresh = document.createElement('button');
  btnRefresh.className = 'btn btn-secondary';
  btnRefresh.textContent = 'Refresh';
  btnRefresh.addEventListener('click', () => renderMetrics(container, groupFilter));
  toolbar.appendChild(btnRefresh);

  container.appendChild(toolbar);

  api.getMetrics(groupFilter, undefined, 24).then((summaries) => {
    if (summaries.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'empty-state';
      empty.textContent = 'No metrics yet. Run some tasks first.';
      container.appendChild(empty);
      return;
    }
    summaries.forEach((s) => renderMetricCard(s, container));
  }).catch((e: Error) => {
    const err = document.createElement('div');
    err.className = 'error-banner';
    err.textContent = e.message;
    container.appendChild(err);
  });
}
