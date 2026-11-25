# UManaged Agents Go SDK

`v0.7.1` uses OpenAPI Generator CLI 7.27.0 and requires Go 1.23+. The module path is unchanged.

```bash
go get github.com/score2021/umanaged-agents-go@v0.7.1
```

Pass the API root URL and your API Key to `NewClient`, or leave them empty to read `MANAGED_AGENTS_API_URL` and `MANAGED_AGENTS_API_KEY` (then the older `MVPP_API_URL` and `MVPP_API_KEY`). There is no built-in address: `NewClient` returns an error when none is set. Use the API root URL without `/v1` or `/console`.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/score2021/umanaged-agents-go"
)

func main() {
	client, err := umanagedagents.NewClient("", "")
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	models, _, err := client.API.ModelsAPI.ListModels(client.Context(ctx)).Execute()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Available models:", len(models.GetData()))
}
```

Use `client.WaitTurn(ctx, sessionID, inputEventID)` to wait for a submitted input, `client.Stream(ctx, sessionID)` to follow live events (open it before sending), and `client.Events(ctx, sessionID, pageSize)` to iterate persisted events. `client.UploadFile(ctx, path)` uploads a file of up to 512 MiB in parts. `EventType`, `EventID` and `EventInputID` read the common fields of any event variant. The Go context controls local waiting.

For an API behind a private CA pass `umanagedagents.WithCAFile(path)` to `NewClient`, or set `MANAGED_AGENTS_CA_FILE`. Verification cannot be turned off. `WithTimeout(d)` changes the per-request limit (default 30 s).

## Changes in v0.7.1

- Response models ignore fields they do not know, so a deployment that adds a field to a response no longer breaks this version (v0.7.0 and earlier fail with `json: unknown field`; v0.6.1 already ignores them). Required fields are still required. The variants of the one oneOf without a discriminator (the `tools` items of an Agent request) stay strict, because the client tells them apart that way (#720).

## Changes in v0.7.0

- New operations `ArchiveEnvironment` and `DeleteEnvironment`; `ListEnvironments` accepts `Archived` (`false` default, `true`, `all`) (#716).
- `Environment` has `available`, `unavailable_reason` (`archived` or `template_retired`) and `archived_at`; creating a Session on an unavailable environment returns 409 `ENVIRONMENT_UNAVAILABLE` (#715). v0.6.0 rejects these fields: upgrade before using a deployment that returns them.

## Changes in v0.6.0

- `ListAgents` accepts `Q` (name or ID contains, case-insensitive) and `Order` (`desc` default, `asc`); `ListSessions` accepts `EnvironmentId`; `ListFiles` accepts `OwnKey` (#658).
- `Session` has `stop_reason` (why an idle Session stopped), `Environment` has `session_count`, Skill list items have `protection_enabled`, `publication_count` and `updated_at`. v0.5.0 rejects these fields: upgrade before using a deployment that returns them.

## Changes in v0.5.0

- `ListModels` accepts `IncludeUnavailable`; each model has `provider`, `input_modalities`, `output_modalities`, `agent_types`, `tool_call`, `available`, `unavailable_reason` and `pricing` (#646). v0.4.0 rejects these fields: upgrade before using a deployment that returns them.

## Changes in v0.4.0

- New operations: chunked uploads (`UploadsAPI`), `DeleteFile`, `GetFileUsage`, usage statistics (`UsageAPI.GetUsage`, `ExportUsage`), storage binding references and credential update. `File` has `session_id` and `import`.
- `client.Stream` reads the SSE event stream as it arrives (the generated `StreamEvents` buffers the whole response and is not suitable for live use).
- `client.UploadFile` and `UploadPart` take the part number as an integer.
- `NewClient(baseURL, apiKey, opts...)`: `WithCAFile` / `MANAGED_AGENTS_CA_FILE` and `WithTimeout`. Existing two-argument calls are unchanged.

## Changes in v0.3.0

- The Agent fields `delivery` and `required_artifacts` were removed from the API and from the generated models. Files the Agent writes to `/mnt/session/outputs` are collected as Session files after every input; list them with the Files API.
- `NewClient("", "")` reads `MANAGED_AGENTS_*` first and no longer falls back to a built-in address.

## Upgrading from the gray preview

The generated API changed in `v0.2.0`. Replace `mvpp.New(key, baseURL)` with `umanagedagents.NewClient(baseURL, key)` and migrate calls such as `ListModelsWithResponse` to the generated request builders shown above. Existing gray clients can pin [v0.1.0](https://github.com/score2021/umanaged-agents-go/tree/v0.1.0) while migrating.
