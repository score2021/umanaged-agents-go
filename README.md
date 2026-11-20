# UManaged Agents Go SDK

`v0.2.0` uses OpenAPI Generator CLI 7.27.0 and requires Go 1.23+. The module path is unchanged.

```bash
go get github.com/score2021/umanaged-agents-go@v0.2.0
```

Set `MVPP_API_KEY` and, for a different deployment, `MVPP_API_URL`. Use the API root URL without `/v1` or `/console`. The default is `https://console.amanager.cn`.

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

## Upgrading from the gray preview

The generated API changed. Replace `mvpp.New(key, baseURL)` with `umanagedagents.NewClient(baseURL, key)` and migrate calls such as `ListModelsWithResponse` to the generated request builders shown above. Existing gray clients can pin [v0.1.0](https://github.com/score2021/umanaged-agents-go/tree/v0.1.0) while migrating. Set `MVPP_API_URL` explicitly for gray environments.
