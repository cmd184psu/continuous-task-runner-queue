import { api, setToken, clearToken, hasToken } from './api.js';
import { renderGroups } from './groups.js';
import { renderTasks } from './tasks.js';
import { renderExecutions } from './executions.js';
import { renderMetrics } from './metrics.js';
import { renderOutput } from './output.js';

function buildNav(): void {
  const nav = document.getElementById('nav');
  if (!nav) return;
  nav.textContent = '';

  const brand = document.createElement('a');
  brand.className = 'nav-brand';
  brand.textContent = 'ctrq';
  brand.href = '#groups';
  nav.appendChild(brand);

  if (hasToken()) {
    const links: Array<{ label: string; hash: string }> = [
      { label: 'Groups', hash: '#groups' },
      { label: 'Tasks', hash: '#tasks' },
      { label: 'Executions', hash: '#executions' },
      { label: 'Metrics', hash: '#metrics' },
    ];
    links.forEach(({ label, hash }) => {
      const a = document.createElement('a');
      a.className = 'nav-link' + (window.location.hash === hash ? ' active' : '');
      a.textContent = label;
      a.href = hash;
      nav.appendChild(a);
    });

    const spacer = document.createElement('div');
    spacer.className = 'nav-spacer';
    nav.appendChild(spacer);

    const btnLogout = document.createElement('button');
    btnLogout.className = 'nav-link';
    btnLogout.textContent = 'Logout';
    btnLogout.addEventListener('click', () => {
      clearToken();
      window.location.hash = '#login';
    });
    nav.appendChild(btnLogout);
  }
}

function renderLogin(container: HTMLElement): void {
  container.textContent = '';

  const wrap = document.createElement('div');
  wrap.className = 'login-wrap';

  const card = document.createElement('div');
  card.className = 'login-card';

  const h2 = document.createElement('h2');
  h2.textContent = 'ctrq';
  card.appendChild(h2);

  const form = document.createElement('form');

  const fg = document.createElement('div');
  fg.className = 'form-group';
  const input = document.createElement('input');
  input.type = 'password';
  input.placeholder = 'passcode';
  input.maxLength = 5;
  input.autofocus = true;
  fg.appendChild(input);
  form.appendChild(fg);

  const btn = document.createElement('button');
  btn.type = 'submit';
  btn.className = 'btn btn-primary';
  btn.style.width = '100%';
  btn.textContent = 'Login';
  form.appendChild(btn);

  const errDiv = document.createElement('div');
  errDiv.className = 'login-error';
  form.appendChild(errDiv);

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errDiv.textContent = '';
    try {
      const result = await api.login(input.value);
      setToken(result.token);
      window.location.hash = '#groups';
    } catch (err: unknown) {
      errDiv.textContent = err instanceof Error ? err.message : 'Login failed';
    }
  });

  card.appendChild(form);
  wrap.appendChild(card);
  container.appendChild(wrap);
}

function route(): void {
  const container = document.getElementById('app');
  if (!container) return;

  buildNav();

  const hash = window.location.hash || '#groups';

  if (!hasToken() && hash !== '#login') {
    window.location.hash = '#login';
    return;
  }

  const [page, param] = hash.slice(1).split('/');

  switch (page) {
    case 'login':
      renderLogin(container);
      break;
    case 'groups':
      renderGroups(container);
      break;
    case 'tasks':
      renderTasks(container, param);
      break;
    case 'executions':
      renderExecutions(container, param);
      break;
    case 'metrics':
      renderMetrics(container, param);
      break;
    case 'output':
      if (param) renderOutput(container, param);
      break;
    default:
      renderGroups(container);
  }
}

window.addEventListener('hashchange', route);
document.addEventListener('DOMContentLoaded', route);
