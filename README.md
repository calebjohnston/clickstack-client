Local ClickStack Setup Guide
==================

The ClickStack documentation is pretty good about telling you what to do as long as your deployment use case scenario fits directly into one of it's six options here: https://clickhouse.com/docs/use-cases/observability/clickstack/deployment

This guide outlines how to configure a *local* ClickStack instance with a client program to provide example input telemetry.

Running Clickhouse
------------------

Installed clickhouse using:
`brew install clickhouse`

This installed the DB server at: `/usr/local/bin/clickhouse`
Which simlinks to: `/usr/local/Caskroom/clickhouse/25.7.4.11-stable/clickhouse-macos`

The clickhouse binary can be started as a client, server, or repl:
- client: `$ clickhouse client`
- server: `$ clickhouse server`
- repl: `$ clickhouse`

The clickhouse server hosts at a TCP socket at port 9000. You can verify that it's up using liof

To inspect tables, run `clickhouse client` and then run `SHOW TABLES` which should produce output like this once in use:
```
SHOW TABLES

Query id: 590a2489-c958-4e07-83bb-e730698c867d

   ┌─name───────────────────────────────┐
1. │ otel_logs                          │
2. │ otel_metrics_exponential_histogram │
3. │ otel_metrics_gauge                 │
4. │ otel_metrics_histogram             │
5. │ otel_metrics_sum                   │
6. │ otel_metrics_summary               │
7. │ otel_traces                        │
8. │ otel_traces_trace_id_ts            │
9. │ otel_traces_trace_id_ts_mv         │
   └────────────────────────────────────┘

9 rows in set. Elapsed: 0.004 sec.
```

Then run `DESCRIBE <insert-table-name>` to get a summary of the table.



Running OpenTelemetry
------------------

Installing an OpenTelemetry collector was more painful. You cannot just use bare-bones OpenTelemetry. You need to use OpenTelemetry with the modules that support your database. In this case, that's clickhouse.

This means you should install from: https://github.com/open-telemetry/opentelemetry-collector-contrib
And **NOT** from: https://github.com/open-telemetry/opentelemetry-collector/

Fortunately, clickhouse support is already there: https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/clickhouseexporter

The best way to install is to build from source using the following instructions: 
```
$ git checkout https://github.com/open-telemetry/opentelemetry-collector-contrib.git
$ cd opentelemetry-collector-contrib
$ make otelcontribcol
```

This will generate a binary at `bin/otelcontribcol_darwin_amd64` on an Apple Silicon Mac.

This binary requires an input configuration file. To configure the OpenTelemetry collector to use clickhouse, you need to supply a YAML file as follows:
```
receivers:
  otlp:
    protocols:
      grpc:
      http:

exporters:
  clickhouse:
    endpoint: tcp://localhost:9000
    database: default
    logs_table_name: otel_logs
    traces_table_name: otel_traces
    metrics_table_name: otel_metrics
    timeout: 5s
    sending_queue:
      enabled: true
      queue_size: 1000
    retry_on_failure:
      enabled: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [clickhouse]
    logs:
      receivers: [otlp]
      exporters: [clickhouse]
    metrics:
      receivers: [otlp]
      exporters: [clickhouse]
```

Now this OpenTelemetry collector will host HTTP and gRPC APIs. The HTTP API is hosted on port 4318. The gRPC API is hosted on port 4317.


Running HyperDX
------------------

To run HyperDX, first ensure clickhouse server and opentelemetry collectors are running. Then just build from source as follows:
```
$ git clone https://github.com/hyperdxio/hyperdx
$ cd hyperdx
$ yarn install
$ yarn run app:dev:local
```

This will run HyperDX in local mode. Local mode will store all of the user configuration in local storage instead of in MongoDB. But that also means your dashboards will get wiped up restarting the app.
This will host the HyperDX web app at: http://localhost:8080
The HyperDX API will be at: http://localhost:8123


Recording Telemetry
------------------
You can use `otel-cli` to write spans to the open telemetry collector as follows:
```
otel-cli span --name test-span --endpoint http://localhost:4318/v1/traces
```

This tool doesn't support metrics or logs. So, it's just way easier to use an SDK. That's what's been done in this repo using the Go SDK.


