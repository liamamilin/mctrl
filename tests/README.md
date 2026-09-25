# Integration tests

Most automated coverage lives beside the package under test so it can access
small internal seams. The real tmux/API/WebSocket integration tests are in
`internal/tmux` and `internal/api` and run as part of:

```sh
go test ./...
```

When tmux is not installed, those tests skip; when tmux is installed, adapter
and WebSocket tests use isolated sessions and clean them up.
