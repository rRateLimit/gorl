# Custom Rate Limiting Algorithms

このディレクトリでは、GoRL でカスタムレートリミッターアルゴリズムを実装および使用する方法を示します。

## 概要

GoRL は拡張可能な設計になっており、独自のレートリミッターアルゴリズムを実装できます。公開された`ratelimiter`パッケージを使用することで、以下のことが可能です：

- カスタムアルゴリズムの実装
- アルゴリズムの登録
- 既存の GoRL ツールでの使用

## クイックスタート

### 1. カスタムアルゴリズムの実装

カスタムアルゴリズムを実装するには、`ratelimiter.RateLimiter`インターフェースを満たす必要があります：

```go
type RateLimiter interface {
    Allow(ctx context.Context) error
    String() string
}
```

### 2. 基本的な実装例

```go
package main

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/rRateLimit/gorl/pkg/ratelimiter"
)

// SimpleCustomLimiter の基本的な実装
type SimpleCustomLimiter struct {
    rate     float64
    interval time.Duration
    lastTime time.Time
    mu       sync.Mutex
}

func NewSimpleCustomLimiter(rate float64) ratelimiter.RateLimiter {
    return &SimpleCustomLimiter{
        rate:     rate,
        interval: time.Duration(float64(time.Second) / rate),
        lastTime: time.Now().Add(-time.Second),
    }
}

func (s *SimpleCustomLimiter) Allow(ctx context.Context) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    now := time.Now()
    nextAllowed := s.lastTime.Add(s.interval)

    if now.Before(nextAllowed) {
        waitDuration := nextAllowed.Sub(now)

        timer := time.NewTimer(waitDuration)
        defer timer.Stop()

        select {
        case <-timer.C:
            s.lastTime = time.Now()
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }
    }

    s.lastTime = now
    return nil
}

func (s *SimpleCustomLimiter) String() string {
    return fmt.Sprintf("Simple Custom Limiter (%.2f req/s)", s.rate)
}
```

### 3. アルゴリズムの登録

```go
func main() {
    // カスタムアルゴリズムを登録
    err := ratelimiter.Register("my-custom", func(rate float64) ratelimiter.RateLimiter {
        return NewSimpleCustomLimiter(rate)
    })
    if err != nil {
        log.Fatalf("Failed to register algorithm: %v", err)
    }

    // 登録されたアルゴリズムを一覧表示
    fmt.Println("Available algorithms:", ratelimiter.List())
}
```

### 4. GoRL での使用

```bash
# カスタムアルゴリズムを使用
gorl -url=https://httpbin.org/get -rate=5 -algorithm=my-custom
```

## 高度な実装例

### 1. Exponential Backoff Limiter

失敗時に指数的にレートを下げるアルゴリズム：

```go
type ExponentialBackoffLimiter struct {
    baseRate      float64
    currentRate   float64
    failureCount  int
    backoffFactor float64
    // ... その他のフィールド
}
```

### 2. Circuit Breaker Limiter

サーキットブレーカーパターンを組み込んだアルゴリズム：

```go
type CircuitBreakerLimiter struct {
    rate             float64
    state            CircuitState
    failureThreshold int
    timeout          time.Duration
    // ... その他のフィールド
}
```

## 実装ガイドライン

### 必須要件

1. **`Allow(ctx context.Context) error`メソッド**

   - コンテキストのキャンセルを適切に処理
   - レート制限に従った待機の実装
   - エラーの適切な返却

2. **`String() string`メソッド**
   - アルゴリズムの説明を返す
   - デバッグとログに使用

### 推奨事項

1. **スレッドセーフ**

   - 複数の goroutine から同時に呼ばれる可能性
   - 適切な mutex や atomic 操作の使用

2. **効率的な実装**

   - 高頻度で呼ばれるメソッド
   - 無駄な計算やメモリ使用を避ける

3. **設定可能性**
   - factory パターンでパラメータを受け取る
   - 環境に応じた調整が可能

## ベストプラクティス

### 1. コンテキスト処理

```go
func (l *CustomLimiter) Allow(ctx context.Context) error {
    // 待機が必要な場合
    timer := time.NewTimer(waitDuration)
    defer timer.Stop()

    select {
    case <-timer.C:
        return nil
    case <-ctx.Done():
        return ctx.Err() // コンテキストのエラーを返す
    }
}
```

### 2. エラーハンドリング

```go
// 適切なエラータイプを返す
if rateLimitExceeded {
    return fmt.Errorf("rate limit exceeded")
}

if circuitOpen {
    return fmt.Errorf("circuit breaker is open")
}
```

### 3. 設定の検証

```go
func NewCustomLimiter(rate float64) ratelimiter.RateLimiter {
    if rate <= 0 {
        rate = 1.0 // デフォルト値
    }

    if rate > 1000 {
        rate = 1000 // 最大値制限
    }

    // ... 実装
}
```

## テスト

カスタムアルゴリズムのテストは重要です：

```go
func TestCustomLimiter(t *testing.T) {
    limiter := NewCustomLimiter(2.0) // 2 req/s

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // 最初のリクエストは即座に許可されるべき
    start := time.Now()
    err := limiter.Allow(ctx)
    elapsed := time.Since(start)

    if err != nil {
        t.Errorf("First request should be allowed: %v", err)
    }

    if elapsed > 100*time.Millisecond {
        t.Errorf("First request took too long: %v", elapsed)
    }

    // 2番目のリクエストは約500ms待機するべき
    start = time.Now()
    err = limiter.Allow(ctx)
    elapsed = time.Since(start)

    if err != nil {
        t.Errorf("Second request should be allowed: %v", err)
    }

    expectedWait := 450 * time.Millisecond
    tolerance := 100 * time.Millisecond

    if elapsed < expectedWait-tolerance || elapsed > expectedWait+tolerance {
        t.Errorf("Second request wait time unexpected: got %v, expected ~%v",
            elapsed, expectedWait)
    }
}
```

## よくある質問

### Q: 既存のアルゴリズムを上書きできますか？

A: いいえ、既存のアルゴリズム名で登録しようとするとエラーになります。

### Q: 実行時にアルゴリズムを変更できますか？

A: 現在は対応していません。アプリケーション起動時に登録する必要があります。

### Q: 複数のパラメータを受け取るアルゴリズムを作成できますか？

A: factory パターンでクロージャを使用することで可能です：

```go
ratelimiter.Register("burst-5", func(rate float64) ratelimiter.RateLimiter {
    return NewBurstLimiter(rate, 5) // burstSize = 5
})

ratelimiter.Register("burst-10", func(rate float64) ratelimiter.RateLimiter {
    return NewBurstLimiter(rate, 10) // burstSize = 10
})
```

## サンプルファイル

- [`custom_algorithm.go`](./custom_algorithm.go) - 完全な実装例
- 実行方法：`go run custom_algorithm.go`

## 関連ドキュメント

- [GoRL README](../README.md)
- [API Documentation](../pkg/ratelimiter/)
- [Built-in Algorithms](../internal/limiter/)
