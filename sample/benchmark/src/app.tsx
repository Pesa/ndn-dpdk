import { Component, createRef, h } from "preact";

import { Benchmark, type BenchmarkOptions, type BenchmarkResult, type ServerEnv } from "./benchmark";
import { BenchmarkOptionsEditor } from "./benchmark-options-editor";
import { ResultTable } from "./result-table";
import { TopologyView } from "./topology-view";

interface State {
  env?: ServerEnv;
  message: string;
  opts: BenchmarkOptions;
  running: boolean;
  results: BenchmarkResult[];
}

export class App extends Component<{}, State> {
  state: State = {
    message: "",
    opts: {
      faceAScheme: "ether",
      faceARxQueues: 1,
      faceBScheme: "ether",
      faceBRxQueues: 1,
      nFwds: 4,
      trafficDir: 2,
      producerKind: "pingserver",
      nProducerThreads: 1,
      nConsumerThreads: 1,
      nFlows: 4,
      interestNameLen: 4,
      dataMatch: "exact",
      payloadLen: 1000,
      segmentEnd: 0,
      warmup: 0,
      duration: 60,
    },
    running: false,
    results: [],
  };

  private abort?: AbortController;
  private readonly $form = createRef<HTMLFormElement>();

  override async componentDidMount() {
    const env = await (await fetch("/env.json")).json();
    this.setState({ env });
  }

  override render() {
    const { env, message, opts, running, results } = this.state;
    if (!env) {
      return <p>loading</p>;
    }
    return (
      <section>
        <TopologyView env={env} opts={opts}/>
        <form ref={this.$form} class="pure-form pure-form-aligned">
          <BenchmarkOptionsEditor opts={opts} disabled={running} onChange={this.handleOptsChange}>
            <div class="pure-controls">
              <button type="button" class="pure-button pure-button-primary" hidden={running} onClick={this.handleStart}>START</button>
              <button type="button" class="pure-button copy-button" hidden={running} onClick={this.handleCopy}>CLI</button>
              <button type="button" class="pure-button stop-button" hidden={!running} onClick={this.handleStop}>STOP</button>
            </div>
          </BenchmarkOptionsEditor>
        </form>
        <p><code>{message}</code></p>
        <ResultTable records={results} running={running}/>
      </section>
    );
  }

  private readonly handleOptsChange = (update: Partial<BenchmarkOptions>) => {
    this.setState(({ opts }) => {
      opts = { ...opts, ...update };
      return { opts };
    });
  };

  private readonly handleStart = () => {
    if (!this.$form.current?.reportValidity()) {
      return;
    }
    this.setState(
      ({ env, opts: { faceARxQueues, faceBRxQueues, nFwds } }) => {
        const demands = {
          F: faceARxQueues + faceBRxQueues + 2 + nFwds,
          A: faceARxQueues + 1 + 2,
          B: faceBRxQueues + 1 + 2,
        };
        const nodeLabels = ["F", "A", "B"] as const;
        if (env!.A_GQLSERVER === env!.B_GQLSERVER) {
          demands.A += demands.B;
          (nodeLabels as unknown as string[]).pop();
        }
        const errs: string[] = [];
        for (const label of nodeLabels) {
          const demand = demands[label];
          const avail = env![`${label}_CORES_PRIMARY`].length;
          if (demand > avail) {
            errs.push(`need ${demand} on ${label} but only ${avail} assigned`);
          }
        }
        if (errs.length > 0) {
          return {
            message: `insufficient CPU cores: ${errs.join(", ")}`,
          };
        }

        this.abort = new AbortController();
        return { running: true, results: [] };
      },
      this.run,
    );
  };

  private readonly run = async () => {
    if (!this.state.running) {
      return;
    }
    const abort = this.abort!;
    try {
      const b = new Benchmark(this.state.env!, this.state.opts, abort.signal);
      this.setState({ message: "starting forwarder and traffic generator" });
      await b.setup();

      let i = 0;
      while (this.state.running) {
        this.setState({ message: `running trial ${++i}` });
        const result = await b.run();
        console.log(result);
        this.setState(({ results }) => ({ results: [...results, result] }));
      }
    } catch (err: unknown) {
      console.error(err);
      if (this.abort === abort) {
        this.setState({ running: false, message: `${err}` });
      }
    }
  };

  private readonly handleStop = () => {
    this.setState(
      () => ({ running: false, message: "stopping" }),
      () => {
        this.abort?.abort();
        this.abort = undefined;
      },
    );
  };

  private readonly handleCopy = async () => {
    await navigator.clipboard.writeText([
      "jq", "-n", `'${JSON.stringify(this.state.opts)}'`, "|",
      "corepack", "pnpm", "-s", "benchmark", "--count", "10",
    ].join(" "));
    this.setState({ message: "CLI command copied to clipboard" });
  };
}
