import { type ActivateFwArgs, type ActivateGenArgs, type FaceLocator, type FetchCounters, type FetcherConfig, type FetchTaskDef, type TgpConfig, gql, GqlClient } from "@usnistgov/ndn-dpdk";
import delay from "delay";

class GqlControlBase {
  constructor(gqlserver: string) {
    this.c = new GqlClient(gqlserver);
  }

  protected readonly c: GqlClient;

  public close(): void {
    this.c.close();
  }

  public async restart(): Promise<void> {
    await this.c.request(gql`mutation { shutdown(restart: true) }`);
    await delay(20000);
    for (let i = 0; i < 30; ++i) {
      try {
        await delay(1000);
        await this.c.request(gql`{ version { version } }`);
        return;
      } catch {}
    }
    throw new Error("restart timeout");
  }

  public activate(role: "forwarder", arg: ActivateFwArgs): Promise<void>;
  public activate(role: "trafficgen", arg: ActivateGenArgs): Promise<void>;
  public async activate(role: string, arg: unknown) {
    await this.c.request(gql`
      mutation activate($arg: JSON!) {
        activate(${role}: $arg)
      }
    `, { arg });
  }

  public async createEthPort(pciAddr: string): Promise<string> {
    const { id } = await this.c.request<{ id: string }>(gql`
      mutation createEthPort($pciAddr: String!) {
        createEthPort(
          driver: PCI
          pciAddr: $pciAddr
          mtu: 9000
          rxFlowQueues: 2
        ) {
          id
        }
      }
    `, { pciAddr }, { key: "createEthPort" });
    return id;
  }
}

export class GqlFwControl extends GqlControlBase {
  public async createFace(locator: FaceLocator): Promise<string> {
    const { id } = await this.c.request<{ id: string }>(gql`
      mutation createFace($locator: JSON!) {
        createFace(locator: $locator) {
          id
        }
      }
    `, { locator }, { key: "createFace" });
    return id;
  }

  public async insertFibEntry(name: string, nexthop: string): Promise<string> {
    const { id } = await this.c.request<{ id: string }>(gql`
      mutation insertFibEntry($name: Name!, $nexthops: [ID!]!) {
        insertFibEntry(name: $name, nexthops: $nexthops) {
          id
        }
      }
    `, {
      name,
      nexthops: [nexthop],
    }, { key: "insertFibEntry" });
    return id;
  }

  public async updateNdt(name: string, value: number): Promise<number> {
    const { index } = await this.c.request<{ index: number }>(gql`
      mutation updateNdt($name: Name!, $value: Int!) {
        updateNdt(name: $name, value: $value) {
          index
        }
      }
    `, {
      name,
      value,
    }, { key: "updateNdt" });
    return index;
  }
}

export class GqlGenControl extends GqlControlBase {
  public async startTrafficGen(face: FaceLocator, producer: TgpConfig, fetcher: FetcherConfig): Promise<{ id: string; face: string; producer: string; fetcher: string }> {
    const result = await this.c.request<{
      id: string;
      face: { id: string };
      producer: { id: string };
      fetcher: { id: string };
    }>(gql`
      mutation startTrafficGen(
        $face: JSON!
        $producer: TgpConfigInput!
        $fetcher: FetcherConfigInput!
      ) {
        startTrafficGen(
          face: $face
          producer: $producer
          fetcher: $fetcher
        ) {
          id
          face { id }
          producer { id }
          fetcher { id }
        }
      }
    `, { face, producer, fetcher }, { key: "startTrafficGen" });
    return {
      id: result.id,
      face: result.face.id,
      producer: result.producer.id,
      fetcher: result.fetcher.id,
    };
  }

  public startFetch(fetcher: string, tasks: readonly FetchTaskDef[]): Promise<string[]> {
    return Promise.all(tasks.map(async (task) => {
      const { id } = await this.c.request<{ id: string }>(gql`
        mutation fetch($fetcher: ID!, $task: FetchTaskDefInput!) {
          fetch(fetcher: $fetcher, task: $task) {
            id
          }
        }
      `, { fetcher, task }, { key: "fetch" });
      return id;
    }));
  }

  public getFetchProgress(ids: readonly string[]): Promise<FetchCounters[]> {
    return Promise.all(ids.map(async (id) => {
      const { counters } = await this.c.request<{ counters: FetchCounters }>(gql`
        query fetchCounters($id: ID!) {
          node(id: $id) {
            ... on FetchTaskContext {
              counters
            }
          }
        }
      `, { id }, { key: "node" });
      return counters;
    }));
  }

  public async stopFetch(ids: readonly string[]): Promise<void> {
    await Promise.all(ids.map((id) => this.c.del(id)));
  }
}
