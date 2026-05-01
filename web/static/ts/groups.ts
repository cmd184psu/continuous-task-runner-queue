import { api, GroupStatus } from './api.js';

function statusBadge(paused: boolean): HTMLElement {
  const span = document.createElement('span');
  span.className = paused ? 'badge badge-yellow' : 'badge badge-green';
  span.textContent = paused ? 'paused' : 'active';
  return span;
}

function renderGroupRow(g: GroupStatus, tbody: HTMLElement, onRefresh: () => void): void {
  const tr = document.createElement('tr');

  const tdName = document.createElement('td');
  tdName.textContent = g.name;

  const tdLimit = document.createElement('td');
  tdLimit.textContent = String(g.pool_limit);

  const tdRunning = document.createElement('td');
  tdRunning.textContent = String(g.running_count);

  const tdStatus = document.createElement('td');
  tdStatus.appendChild(statusBadge(g.paused));

  const tdActions = document.createElement('td');
  const btnGroup = document.createElement('div');
  btnGroup.className = 'btn-group';

  if (g.paused) {
    const btnResume = document.createElement('button');
    btnResume.className = 'btn btn-secondary btn-sm';
    btnResume.textContent = 'Resume';
    btnResume.addEventListener('click', async () => {
      try { await api.resumeGroup(g.name); onRefresh(); } catch (e) { showError(String(e)); }
    });
    btnGroup.appendChild(btnResume);
  } else {
    const btnPause = document.createElement('button');
    btnPause.className = 'btn btn-secondary btn-sm';
    btnPause.textContent = 'Pause';
    btnPause.addEventListener('click', async () => {
      try { await api.pauseGroup(g.name); onRefresh(); } catch (e) { showError(String(e)); }
    });
    btnGroup.appendChild(btnPause);
  }

  const btnEdit = document.createElement('button');
  btnEdit.className = 'btn btn-secondary btn-sm';
  btnEdit.textContent = 'Edit';
  btnEdit.addEventListener('click', () => showEditModal(g, onRefresh));
  btnGroup.appendChild(btnEdit);

  const btnDel = document.createElement('button');
  btnDel.className = 'btn btn-danger btn-sm';
  btnDel.textContent = 'Delete';
  btnDel.addEventListener('click', async () => {
    if (!confirm('Delete group "' + g.name + '"? This fails if tasks exist.')) return;
    try { await api.deleteGroup(g.name); onRefresh(); } catch (e) { showError(String(e)); }
  });
  btnGroup.appendChild(btnDel);

  tdActions.appendChild(btnGroup);
  tr.appendChild(tdName);
  tr.appendChild(tdLimit);
  tr.appendChild(tdRunning);
  tr.appendChild(tdStatus);
  tr.appendChild(tdActions);
  tbody.appendChild(tr);
}

function showCreateModal(onRefresh: () => void): void {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';

  const modal = document.createElement('div');
  modal.className = 'modal';

  const title = document.createElement('h2');
  title.textContent = 'Create Group';
  modal.appendChild(title);

  const fgName = document.createElement('div');
  fgName.className = 'form-group';
  const lblName = document.createElement('label');
  lblName.textContent = 'Name';
  const inputName = document.createElement('input');
  inputName.type = 'text';
  inputName.placeholder = 'my-group';
  fgName.appendChild(lblName);
  fgName.appendChild(inputName);
  modal.appendChild(fgName);

  const fgLimit = document.createElement('div');
  fgLimit.className = 'form-group';
  const lblLimit = document.createElement('label');
  lblLimit.textContent = 'Pool Limit';
  const inputLimit = document.createElement('input');
  inputLimit.type = 'number';
  inputLimit.value = '1';
  inputLimit.min = '1';
  fgLimit.appendChild(lblLimit);
  fgLimit.appendChild(inputLimit);
  modal.appendChild(fgLimit);

  const actions = document.createElement('div');
  actions.className = 'form-actions';

  const btnCreate = document.createElement('button');
  btnCreate.className = 'btn btn-primary';
  btnCreate.textContent = 'Create';
  btnCreate.addEventListener('click', async () => {
    try {
      await api.createGroup({ name: inputName.value.trim(), pool_limit: Number(inputLimit.value), allowed_types: [] });
      overlay.remove();
      onRefresh();
    } catch (e) { showError(String(e)); }
  });

  const btnCancel = document.createElement('button');
  btnCancel.className = 'btn btn-secondary';
  btnCancel.textContent = 'Cancel';
  btnCancel.addEventListener('click', () => overlay.remove());

  actions.appendChild(btnCreate);
  actions.appendChild(btnCancel);
  modal.appendChild(actions);

  overlay.appendChild(modal);
  overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });
  document.body.appendChild(overlay);
}

function showEditModal(g: GroupStatus, onRefresh: () => void): void {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';

  const modal = document.createElement('div');
  modal.className = 'modal';

  const title = document.createElement('h2');
  title.textContent = 'Edit Group: ' + g.name;
  modal.appendChild(title);

  const fgLimit = document.createElement('div');
  fgLimit.className = 'form-group';
  const lblLimit = document.createElement('label');
  lblLimit.textContent = 'Pool Limit';
  const inputLimit = document.createElement('input');
  inputLimit.type = 'number';
  inputLimit.value = String(g.pool_limit);
  inputLimit.min = '1';
  fgLimit.appendChild(lblLimit);
  fgLimit.appendChild(inputLimit);
  modal.appendChild(fgLimit);

  const actions = document.createElement('div');
  actions.className = 'form-actions';

  const btnSave = document.createElement('button');
  btnSave.className = 'btn btn-primary';
  btnSave.textContent = 'Save';
  btnSave.addEventListener('click', async () => {
    try {
      await api.updateGroup(g.name, { pool_limit: Number(inputLimit.value) });
      overlay.remove();
      onRefresh();
    } catch (e) { showError(String(e)); }
  });

  const btnCancel = document.createElement('button');
  btnCancel.className = 'btn btn-secondary';
  btnCancel.textContent = 'Cancel';
  btnCancel.addEventListener('click', () => overlay.remove());

  actions.appendChild(btnSave);
  actions.appendChild(btnCancel);
  modal.appendChild(actions);

  overlay.appendChild(modal);
  overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });
  document.body.appendChild(overlay);
}

let errorTimer: ReturnType<typeof setTimeout> | null = null;
function showError(msg: string): void {
  let banner = document.getElementById('global-error') as HTMLElement | null;
  if (!banner) {
    banner = document.createElement('div');
    banner.id = 'global-error';
    banner.className = 'error-banner';
    const app = document.getElementById('app');
    if (app) app.prepend(banner);
  }
  banner.textContent = msg;
  if (errorTimer) clearTimeout(errorTimer);
  errorTimer = setTimeout(() => banner?.remove(), 5000);
}

export function renderGroups(container: HTMLElement): void {
  container.textContent = '';

  const toolbar = document.createElement('div');
  toolbar.className = 'toolbar';

  const h1 = document.createElement('h1');
  h1.textContent = 'Groups';
  toolbar.appendChild(h1);

  const spacer = document.createElement('div');
  spacer.className = 'toolbar-spacer';
  toolbar.appendChild(spacer);

  const refresh = (): void => { renderGroups(container); };

  const btnNew = document.createElement('button');
  btnNew.className = 'btn btn-primary';
  btnNew.textContent = '+ New Group';
  btnNew.addEventListener('click', () => showCreateModal(refresh));
  toolbar.appendChild(btnNew);

  const btnRefresh = document.createElement('button');
  btnRefresh.className = 'btn btn-secondary';
  btnRefresh.textContent = 'Refresh';
  btnRefresh.addEventListener('click', refresh);
  toolbar.appendChild(btnRefresh);

  container.appendChild(toolbar);

  api.listGroups().then((groups) => {
    if (groups.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'empty-state';
      empty.textContent = 'No groups yet. Create one to get started.';
      container.appendChild(empty);
      return;
    }

    const card = document.createElement('div');
    card.className = 'card';

    const table = document.createElement('table');
    const thead = document.createElement('thead');
    const headerRow = document.createElement('tr');
    ['Name', 'Pool Limit', 'Running', 'Status', 'Actions'].forEach((h) => {
      const th = document.createElement('th');
      th.textContent = h;
      headerRow.appendChild(th);
    });
    thead.appendChild(headerRow);
    table.appendChild(thead);

    const tbody = document.createElement('tbody');
    groups.forEach((g) => renderGroupRow(g, tbody, refresh));
    table.appendChild(tbody);

    card.appendChild(table);
    container.appendChild(card);
  }).catch((e: Error) => showError(e.message));
}
