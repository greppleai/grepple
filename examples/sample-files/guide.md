# Search Guide

Overview of how the demo service handles queries.

## Configuration

Set the `timeout` before calling `NewServer`.

### TLS

Enable TLS in production. The `minVersion` should be at least 1.2.

## Querying

Use the `search` feature to run a query, then narrow with a glob.

### Limits

The `maxFiles` limit protects the shard from huge result sets.
