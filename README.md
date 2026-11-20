# UManaged Agents Go SDK

```bash
go get github.com/score2021/umanaged-agents-go@v0.1.0
```

本预览版需要 Go 1.25.5 或更高版本，默认连接
`http://managed-agents-gray.modelverse.cn`。调用环境需要能访问灰度入口，并拥有可用的 Astraflow Key。HTTP 会明文传输 Key，适用于受信任网络中的灰度测试。

使用 `mvpp.New("", "")` 读取 `MVPP_API_KEY` 并使用默认 API 地址。
可用 `MVPP_API_URL` 覆盖默认值，也可传入 `mvpp.New(key, baseURL)` 显式覆盖。
地址填写服务根地址，无需附加 `/v1`。

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	mvpp "github.com/score2021/umanaged-agents-go"
)

func main() {
	client, err := mvpp.New("", "")
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	models, err := client.ListModelsWithResponse(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if models.JSON200 == nil {
		log.Fatalf("list models: HTTP %d", models.StatusCode())
	}
	fmt.Printf("Available models: %d\n", len(models.JSON200.Data))
}
```
