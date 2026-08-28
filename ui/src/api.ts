// Calls into the plugin's Go backend. Stash's runPluginOperation mutation
// spawns the binary, hands it the args and returns whatever the operation put
// in its output — the payloads in types.ts.
import { Apollo, StashService } from "./plugin";
import type { AwardsPayload, SourceName, SyncPayload } from "./types";

// Stash addresses the plugin by its YAML file name.
export const PLUGIN_ID = "awards";

const RUN_OPERATION = Apollo.gql`
  mutation RunPluginOperation($plugin_id: ID!, $args: Map) {
    runPluginOperation(plugin_id: $plugin_id, args: $args)
  }
`;

async function run<T>(args: Record<string, unknown>): Promise<T> {
  const client = StashService.getClient();
  const result = await client.mutate({
    mutation: RUN_OPERATION,
    variables: { plugin_id: PLUGIN_ID, args },
  });
  if (result.errors?.length) {
    throw new Error(result.errors[0].message);
  }
  return result.data.runPluginOperation as T;
}

// getAwards reads everything stored for a performer. By default the backend
// syncs whatever has come due first; sync=false reads the database only.
export function getAwards(
  performerId: string,
  opts: { sync?: boolean; force?: boolean } = {}
): Promise<AwardsPayload> {
  return run<AwardsPayload>({
    mode: "getAwards",
    performerId,
    sync: opts.sync ?? true,
    force: opts.force ?? false,
  });
}

// syncSource re-scrapes one source for one performer right now.
export function syncSource(
  performerId: string,
  source: SourceName
): Promise<SyncPayload> {
  return run<SyncPayload>({
    mode: "sync",
    performerId,
    source,
    force: true,
  });
}
