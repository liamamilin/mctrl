export type HostStatus = 'online' | 'reconnecting' | 'offline' | 'unknown';

export interface Host {
  name: string;
  hostname: string;
  status: HostStatus;
  remote_ready: boolean;
  transport?: string;
  public_url?: string;
  version?: string;
  profile?: string;
}

export interface Project {
  id: string;
  name: string;
  path: string;
  default_runner?: string;
}

export interface Runner {
  id: string;
  name: string;
  available: boolean;
}

export interface ActivePane {
  command?: string;
  cwd?: string;
  width?: number;
  height?: number;
}

export interface Session {
  id: string;
  name: string;
  windows: number;
  panes: number;
  attached: boolean;
  active_pane?: ActivePane;
  active_command?: string;
  cwd?: string;
  managed_work?: ManagedWork;
  work?: ManagedWork;
}

export interface ManagedWork {
  id: string;
  request_id?: string;
  project_id?: string;
  runner_id?: string;
  session_id?: string;
  session_name?: string;
  state: string;
  created_at?: string;
  started_at?: string;
  finished_at?: string;
  runner_pid?: number;
  child_pid?: number;
  exit_code?: number;
  exit_signal?: number;
  keep_awake?: boolean;
  launch_stage?: string;
  termination_reason?: string;
  recovery_status?: string;
  prompt_delivery?: string;
  error_code?: string;
  error_message?: string;
}

export interface Preview {
  lines: string[];
  captured_at?: string;
}

export interface PairedDevice {
  id: string;
  name: string;
  display_name?: string;
  created_at?: string;
  last_seen?: string;
  revoked_at?: string;
}

export interface LaunchRequest {
  request_id: string;
  project_id: string;
  runner_id: string;
  prompt: string;
}

export interface LaunchResult {
  work?: ManagedWork;
  session?: Session;
  work_id?: string;
  session_id?: string;
  request_id?: string;
}

export interface PromptRequest {
  request_id: string;
  text: string;
}
