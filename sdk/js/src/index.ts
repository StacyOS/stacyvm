/**
 * StacyVM TypeScript SDK -- client for the StacyVM sandbox orchestrator.
 *
 * @example
 * ```ts
 * import { Client } from "stacyvm";
 *
 * const client = new Client();
 * const sandbox = await client.spawn({ image: "alpine:latest" });
 *
 * const result = await sandbox.exec("echo hello");
 * console.log(result.stdout);
 *
 * await sandbox.destroy();
 * ```
 *
 * @packageDocumentation
 */

// -- Core classes -----------------------------------------------------------
export { Client } from "./client.js";
export { Sandbox } from "./sandbox.js";
export { TemplateManager } from "./templates.js";

// -- Error classes ----------------------------------------------------------
export {
  ForgevmError,
  SandboxNotFoundError,
  ProviderError,
  ConnectionError,
  handleResponse,
} from "./errors.js";

// -- Streaming utilities ----------------------------------------------------
export { parseNDJSON } from "./streaming.js";

// -- Type definitions -------------------------------------------------------
export type {
  SandboxConfig,
  SpawnOptions,
  ExecOptions,
  ExecResult,
  SandboxInfo,
  SandboxState,
  Template,
  TemplateConfig,
  TemplateSpawnOverrides,
  ProviderInfo,
  QuotaSummary,
  HealthInfo,
  SpawnAdmissionDecision,
  StreamChunk,
  FileInfo,
  ForgevmClientOptions,
  ApiErrorBody,
} from "./types.js";
