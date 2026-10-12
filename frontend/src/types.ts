// Shapes returned by the node's HTTP API, derived from the OpenAPI document
// (backend/internal/httpapi/openapi.json) so they cannot drift from the
// handlers. Regenerate with `npm run codegen`.
import type { components } from "./api-types.gen";

export type KeyValue = components["schemas"]["KeyValue"];
export type Status = components["schemas"]["Status"];

// /cluster is served by backend/internal/webui and is not part of the OpenAPI
// document: the local node plus its peer ids.
export interface ClusterInfo {
  self: string;
  peers: string[];
}
