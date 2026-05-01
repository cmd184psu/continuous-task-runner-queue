export interface GroupConfig {
  name: string;
  pool_limit: number;
  allowed_types: string[];
}

export interface Group extends GroupConfig {
  paused: boolean;
  paused_at?: string;
  paused_by?: string;
  created_at: string;
  updated_at: string;
}

export interface GroupStatus extends Group {
  running_count: number;
}

export interface Task {
  id: number;
  name: string;
  group_name: string;
  enabled: boolean;
  paused: boolean;
  priority: number;
  cooldown_seconds: number;
  repeat: boolean;
  task_type: string;
  args: string;
  sudo: boolean;
  output_file?: string;
  created_at: string;
  updated_at: string;
}

export interface TaskExecution {
  id: number;
  task_id: number;
  task_name?: string;
  scheduled_at?: string;
  started_at?: string;
  finished_at?: string;
  status: string;
  error_message?: string;
  worker_id?: string;
  duration_ms?: number;
  schedule_delay_ms?: number;
}

export interface MetricSummary {
  task_name: string;
  group_name: string;
  success_count: number;
  failed_count: number;
  avg_duration_ms?: number;
  min_duration_ms?: number;
  max_duration_ms?: number;
  avg_schedule_delay_ms?: number;
  last_execution?: string;
}

let _token: string | null = sessionStorage.getItem('ctrq-token');

export function setToken(token: string): void {
  _token = token;
  sessionStorage.setItem('ctrq-token', token);
}

export function clearToken(): void {
  _token = null;
  sessionStorage.removeItem('ctrq-token');
}

export function hasToken(): boolean {
  return _token !== null && _token.length > 0;
}

async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const existingHeaders = options.headers as Record<string, string> | undefined;
  if (existingHeaders) {
    Object.assign(headers, existingHeaders);
  }
  if (_token) {
    headers['Authorization'] = 'Bearer ' + _token;
  }
  const resp = await fetch(path, { ...options, headers });
  if (resp.status === 401) {
    clearToken();
    window.location.hash = '#login';
    throw new Error('unauthorized');
  }
  if (!resp.ok) {
    let errMsg = resp.statusText;
    try {
      const body = await resp.json() as { error?: string };
      if (body.error) errMsg = body.error;
    } catch { /* ignore */ }
    throw new Error(errMsg);
  }
  if (resp.status === 204) return undefined as unknown as T;
  return resp.json() as Promise<T>;
}

export const api = {
  login(passcode: string) {
    return apiFetch<{ token: string }>('/api/auth/token', {
      method: 'POST',
      body: JSON.stringify({ passcode }),
    });
  },

  health() {
    return apiFetch<{ status: string }>('/api/health');
  },

  // Groups
  listGroups() { return apiFetch<GroupStatus[]>('/api/groups'); },
  getGroup(name: string) { return apiFetch<GroupStatus>('/api/groups/' + name); },
  createGroup(g: Partial<GroupConfig>) {
    return apiFetch<Group>('/api/groups', { method: 'POST', body: JSON.stringify(g) });
  },
  updateGroup(name: string, updates: Partial<GroupConfig>) {
    return apiFetch<Group>('/api/groups/' + name, { method: 'PUT', body: JSON.stringify(updates) });
  },
  deleteGroup(name: string) {
    return apiFetch<void>('/api/groups/' + name, { method: 'DELETE' });
  },
  pauseGroup(name: string) {
    return apiFetch<void>('/api/groups/' + name + '/pause', { method: 'POST' });
  },
  resumeGroup(name: string) {
    return apiFetch<void>('/api/groups/' + name + '/resume', { method: 'POST' });
  },

  // Tasks
  listTasks(group?: string) {
    const q = group ? '?group=' + encodeURIComponent(group) : '';
    return apiFetch<Task[]>('/api/tasks' + q);
  },
  getTask(name: string) { return apiFetch<Task>('/api/tasks/' + name); },
  addTask(task: Partial<Task>) {
    return apiFetch<Task>('/api/tasks', { method: 'POST', body: JSON.stringify(task) });
  },
  updateTask(name: string, updates: Partial<Task>) {
    return apiFetch<Task>('/api/tasks/' + name, { method: 'PUT', body: JSON.stringify(updates) });
  },
  deleteTask(name: string) {
    return apiFetch<void>('/api/tasks/' + name, { method: 'DELETE' });
  },
  pauseTask(name: string) {
    return apiFetch<void>('/api/tasks/' + name + '/pause', { method: 'POST' });
  },
  resumeTask(name: string) {
    return apiFetch<void>('/api/tasks/' + name + '/resume', { method: 'POST' });
  },
  enqueueTask(name: string) {
    return apiFetch<{ execution_id: number }>('/api/tasks/' + name + '/enqueue', { method: 'POST' });
  },

  // Executions
  listExecutions(taskName?: string, limit = 50) {
    const params = new URLSearchParams({ limit: String(limit) });
    if (taskName) params.set('task', taskName);
    return apiFetch<TaskExecution[]>('/api/executions?' + params);
  },

  // Metrics
  getMetrics(group?: string, task?: string, hours = 24) {
    const params = new URLSearchParams({ hours: String(hours) });
    if (group) params.set('group', group);
    if (task) params.set('task', task);
    return apiFetch<MetricSummary[]>('/api/metrics?' + params);
  },
};
