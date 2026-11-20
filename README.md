# UManaged Agents Go SDK

`v0.3.0` uses OpenAPI Generator CLI 7.27.0 and requires Go 1.23+. The module path is unchanged.

```bash
go get github.com/score2021/umanaged-agents-go@v0.3.0
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

Use `client.WaitTurn(ctx, sessionID, inputEventID)` to wait for a submitted input, and `client.Events(ctx, sessionID, pageSize)` to iterate persisted events. The Go context controls local waiting.

## Changes in v0.3.0

- The Agent fields `delivery` and `required_artifacts` were removed from the API and from the generated models. Files the Agent writes to `/mnt/session/outputs` are collected as Session files after every input; list them with the Files API.
- `NewClient("", "")` reads `MANAGED_AGENTS_*` first and no longer falls back to a built-in address.

## Upgrading from the gray preview

The generated API changed in `v0.2.0`. Replace `mvpp.New(key, baseURL)` with `umanagedagents.NewClient(baseURL, key)` and migrate calls such as `ListModelsWithResponse` to the generated request builders shown above. Existing gray clients can pin [v0.1.0](https://github.com/score2021/umanaged-agents-go/tree/v0.1.0) while migrating.
