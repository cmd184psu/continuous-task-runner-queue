import { api, Task, GroupStatus } from './api.js';

function taskStatusBadge(task: Task): HTMLElement {
  const span = document.createElement('span');
  if (!task.enabled) {
    span.className = 'badge badge-muted';
    span.textContent = 'disabled';
  } else if (task.paused) {
    span.className = 'badge badge-yellow';
    span.textContent = 'paused';
  } else {
    span.className = 'badge badge-green';
    span.textContent = 'active';
  }
  return span;
}

function renderTaskRow(task: Task, tbody: HTMLElement, onRefresh: () => void): void {
  const tr = document.createElement('tr');

  const cells: HTMLElement[] = [];

  const tdName = document.createElement('td');
  tdName.textContent = task.name;
  cells.push(tdName);

  const tdGroup = document.createElement('td');
  tdGroup.textContent = task.group_name;
  cells.push(tdGroup);

  const tdType = document.createElement('td');
  const typeSpan = document.createElement('code');
  typeSpan.textContent = task.task_type;
  tdType.appendChild(typeSpan);
  cells.push(tdType);

  const tdStatus = document.createElement('td');
  tdStatus.appendChild(taskStatusBadge(task));
  cells.push(tdStatus);

  const tdRepeat = document.createElement('td');
  tdRepeat.textContent = task.repeat ? (task.cooldown_seconds + 's') : 'once';
  cells.push(tdRepeat);

  const tdActions = document.createElement('td');
  const btnGroup = document.createElement('div');
  btnGroup.className = 'btn-group';

  const btnEnqueue = document.createElement('button');
  btnEnqueue.className = 'btn btn-primary btn-sm';
  btnEnqueue.textContent = 'Run now';
  btnEnqueue.addEventListener('click', async () => {
    try {
      const res = await api.enqueueTask(task.name);
      window.location.hash = '#output/' + res.execution_id;
    } catch (e) { alert(String(e)); }
  });
  btnGroup.appendChild(btnEnqueue);

  const btnTogglePause = document.createElement('button');
  btnTogglePause.className = 'btn btn-secondary btn-sm';
  btnTogglePause.textContent = task.paused ? 'Resume' : 'Pause';
  btnTogglePause.addEventListener('click', async () => {
    try {
      if (task.paused) await api.resumeTask(task.name);
      else await api.pauseTask(task.name);
      onRefresh();
    } catch (e) { alert(String(e)); }
  });
  btnGroup.appendChild(btnTogglePause);

  const btnDel = document.createElement('button');
  btnDel.className = 'btn btn-danger btn-sm';
  btnDel.textContent = 'Delete';
  btnDel.addEventListener('click', async () => {
    if (!confirm('Delete task "' + task.name + '"?')) return;
    try { await api.deleteTask(task.name); onRefresh(); } catch (e) { alert(String(e)); }
  });
  btnGroup.appendChild(btnDel);

  tdActions.appendChild(btnGroup);
  cells.push(tdActions);

  cells.forEach((c) => tr.appendChild(c));
  tbody.appendChild(tr);
}

function showAddTaskModal(groups: GroupStatus[], onRefresh: () => void): void {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';

  const modal = document.createElement('div');
  modal.className = 'modal';

  const title = document.createElement('h2');
  title.textContent = 'Add Task';
  modal.appendChild(title);

  function formGroup(label: string, input: HTMLElement): HTMLElement {
    const fg = document.createElement('div');
    fg.className = 'form-group';
    const lbl = document.createElement('label');
    lbl.textContent = label;
    fg.appendChild(lbl);
    fg.appendChild(input);
    return fg;
  }

  const inputName = document.createElement('input');
  inputName.type = 'text';
  inputName.placeholder = 'my-task';
  modal.appendChild(formGroup('Name *', inputName));

  const selectGroup = document.createElement('select');
  groups.forEach((g) => {
    const opt = document.createElement('option');
    opt.value = g.name;
    opt.textContent = g.name;
    selectGroup.appendChild(opt);
  });
  modal.appendChild(formGroup('Group *', selectGroup));

  const argsTemplates: Record<string, string> = {
    shell:     '{"shell":"echo hello"}',
    exec:      '{"command":"ls","args":["-la"],"workdir":"/tmp"}',
    script:    '{"path":"/opt/scripts/run.sh","args":["--dry"]}',
    migration: '{"name":"0001_example"}',
  };

  const selectType = document.createElement('select');
  ['shell', 'exec', 'script', 'migration'].forEach((t) => {
    const opt = document.createElement('option');
    opt.value = t;
    opt.textContent = t;
    selectType.appendChild(opt);
  });
  modal.appendChild(formGroup('Task Type', selectType));

  const textArgs = document.createElement('textarea');
  textArgs.rows = 3;
  textArgs.value = argsTemplates['shell'];
  modal.appendChild(formGroup('Args (JSON)', textArgs));

  selectType.addEventListener('change', () => {
    textArgs.value = argsTemplates[selectType.value] ?? '{}';
  });

  const row = document.createElement('div');
  row.className = 'form-row';

  const inputPriority = document.createElement('input');
  inputPriority.type = 'number';
  inputPriority.value = '50';
  row.appendChild(formGroup('Priority', inputPriority));

  const inputCooldown = document.createElement('input');
  inputCooldown.type = 'number';
  inputCooldown.value = '0';
  row.appendChild(formGroup('Cooldown (sec)', inputCooldown));
  modal.appendChild(row);

  const checkRepeat = document.createElement('input');
  checkRepeat.type = 'checkbox';
  checkRepeat.id = 'task-repeat';
  const lblRepeat = document.createElement('label');
  lblRepeat.className = 'checkbox-label form-group';
  lblRepeat.appendChild(checkRepeat);
  const lblText = document.createElement('span');
  lblText.textContent = 'Repeat';
  lblRepeat.appendChild(lblText);
  modal.appendChild(lblRepeat);

  const checkSudo = document.createElement('input');
  checkSudo.type = 'checkbox';
  const lblSudo = document.createElement('label');
  lblSudo.className = 'checkbox-label form-group';
  lblSudo.appendChild(checkSudo);
  const lblSudoText = document.createElement('span');
  lblSudoText.textContent = 'Run with sudo';
  lblSudo.appendChild(lblSudoText);
  modal.appendChild(lblSudo);

  const inputOutputFile = document.createElement('input');
  inputOutputFile.type = 'text';
  inputOutputFile.placeholder = '/var/log/ctrq/{task}.log  (optional; {exec_id} also supported)';
  modal.appendChild(formGroup('Output File', inputOutputFile));

  const actions = document.createElement('div');
  actions.className = 'form-actions';

  const errDiv = document.createElement('div');
  errDiv.className = 'error-banner';
  errDiv.style.display = 'none';

  const btnAdd = document.createElement('button');
  btnAdd.className = 'btn btn-primary';
  btnAdd.textContent = 'Add Task';
  btnAdd.addEventListener('click', async () => {
    errDiv.style.display = 'none';
    try {
      const newTask: Partial<Task> = {
        name: inputName.value.trim(),
        group_name: selectGroup.value,
        task_type: selectType.value,
        args: textArgs.value.trim() || '{}',
        priority: Number(inputPriority.value),
        cooldown_seconds: Number(inputCooldown.value),
        repeat: checkRepeat.checked,
        sudo: checkSudo.checked,
        enabled: true,
      };
      const outFile = inputOutputFile.value.trim();
      if (outFile) newTask.output_file = outFile;
      await api.addTask(newTask);
      overlay.remove();
      onRefresh();
    } catch (e) {
      errDiv.textContent = String(e);
      errDiv.style.display = 'block';
    }
  });

  const btnCancel = document.createElement('button');
  btnCancel.className = 'btn btn-secondary';
  btnCancel.textContent = 'Cancel';
  btnCancel.addEventListener('click', () => overlay.remove());

  actions.appendChild(btnAdd);
  actions.appendChild(btnCancel);
  modal.appendChild(errDiv);
  modal.appendChild(actions);

  overlay.appendChild(modal);
  overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });
  document.body.appendChild(overlay);
}

export function renderTasks(container: HTMLElement, groupFilter?: string): void {
  container.textContent = '';

  const refresh = (): void => { renderTasks(container, groupFilter); };

  const toolbar = document.createElement('div');
  toolbar.className = 'toolbar';

  const h1 = document.createElement('h1');
  h1.textContent = groupFilter ? 'Tasks — ' + groupFilter : 'Tasks';
  toolbar.appendChild(h1);

  const spacer = document.createElement('div');
  spacer.className = 'toolbar-spacer';
  toolbar.appendChild(spacer);

  const btnRefresh = document.createElement('button');
  btnRefresh.className = 'btn btn-secondary';
  btnRefresh.textContent = 'Refresh';
  btnRefresh.addEventListener('click', refresh);
  toolbar.appendChild(btnRefresh);

  container.appendChild(toolbar);

  Promise.all([api.listTasks(groupFilter), api.listGroups()]).then(([tasks, groups]) => {
    const btnNew = document.createElement('button');
    btnNew.className = 'btn btn-primary';
    btnNew.textContent = '+ New Task';
    btnNew.addEventListener('click', () => showAddTaskModal(groups, refresh));
    toolbar.insertBefore(btnNew, btnRefresh);

    if (tasks.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'empty-state';
      empty.textContent = 'No tasks yet.';
      container.appendChild(empty);
      return;
    }

    const card = document.createElement('div');
    card.className = 'card';

    const table = document.createElement('table');
    const thead = document.createElement('thead');
    const headerRow = document.createElement('tr');
    ['Name', 'Group', 'Type', 'Status', 'Schedule', 'Actions'].forEach((h) => {
      const th = document.createElement('th');
      th.textContent = h;
      headerRow.appendChild(th);
    });
    thead.appendChild(headerRow);
    table.appendChild(thead);

    const tbody = document.createElement('tbody');
    tasks.forEach((t) => renderTaskRow(t, tbody, refresh));
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
