# ADR-001: Event sink for the local CDC lab

Status: PROPOSED

## Context

M1 must prove that an insert into `public.orders` reaches a sink that we can inspect. The lab uses Debezium Server and should remain small enough to run on a laptop. Debezium Server documents an HTTP client sink (`debezium.sink.type=http`, `debezium.sink.http.url`): https://debezium.io/documentation/reference/3.5/operations/debezium-server.html

## Options

1. HTTP client sink to a small Go receiver. This directly satisfies the receiver-log check and needs no broker. The receiver must accept HTTP requests and log their arrival and payload.
2. A sink that writes to a local file or console. This would use fewer application components, but would not exercise the requested Go receiver and would change the M1 acceptance check.
3. A broker sink with a separate consumer. This resembles common deployments, but adds a broker and consumer configuration that do not help answer the WAL-retention question.

## Recommendation

Use the built-in HTTP client sink with a Go HTTP receiver. Send one event per request, log a receive timestamp and payload, and return success only after the receiver accepts the request. Keep Debezium's documented retry behavior visible in configuration so receiver failures do not look like successful delivery. The receiver log demonstrates event arrival; it is not a durable event store.
