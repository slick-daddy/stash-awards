// Shapes of the JSON the Go backend replies with. Kept in one place so a
// payload change in internal/ has exactly one counterpart here.

export type SourceName = "iafd" | "aia";

export type AwardResult = "won" | "nominated" | "inducted";

export interface Award {
  id: number;
  performerId: string;
  source: SourceName;
  organization: string;
  awardName: string;
  category?: string;
  year: number;
  event?: string;
  result: AwardResult;
  sourceUrl?: string;
  associatedMovie?: string;
  associatedMovieUrl?: string;
  associatedMovieYear?: number;
  lastScraped: string;
}

// internal/ops.SourceView
export interface SourceView {
  source: SourceName;
  label: string;
  enabled: boolean;
  url?: string;
  urlResolvedAt?: string;
  lastSynced?: string;
  nextSyncAfter?: string;
  error?: string;
  count: number;
  awards?: Award[];
}

// internal/config.Settings
export interface Settings {
  autoSyncEnabled: boolean;
  syncIntervalDays: number;
  iafdEnabled: boolean;
  aiaEnabled: boolean;
  iafdDelayMs: number;
  aiaDelayMs: number;
}

// internal/ops.AwardsPayload
export interface AwardsPayload {
  performerId: string;
  performerName?: string;
  settings: Settings;
  sources: SourceView[];
  total: number;
  synced?: SyncResult[];
  warning?: string;
}

// internal/syncer.Result
export type SyncStatus =
  | "synced"
  | "skipped"
  | "disabled"
  | "ambiguous"
  | "unresolved"
  | "failed";

export interface SyncResult {
  performerId: string;
  source: SourceName;
  status: SyncStatus;
  url?: string;
  origin?: string;
  awards: number;
  message?: string;
}

// internal/ops.SyncPayload
export interface SyncPayload {
  performerId: string;
  results: SyncResult[];
}
