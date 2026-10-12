// Shapes returned by the node's HTTP API (see backend/internal/httpapi).
export interface Status {
  id: string;
  role: string;
  term: number;
  leader: string;
  commitIndex: number;
  lastApplied: number;
  peers: number;
}

// /cluster is served by backend/internal/webui: the local node plus its peer ids.
export interface ClusterInfo {
  self: string;
  peers: string[];
}
