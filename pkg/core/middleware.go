package core

import (
	"fmt"
	"log"
	"time"
)

// StageRunner 是运行单个 stage 的函数签名
type StageRunner func(rc *RunContext, st Stage) StageResult

// Middleware 是中间件函数，接收一个 StageRunner 返回一个包装过的 StageRunner
type Middleware func(next StageRunner) StageRunner

// Chain 将多个中间件链接起来
func Chain(mws ...Middleware) Middleware {
	return func(final StageRunner) StageRunner {
		wrapped := final
		for i := len(mws) - 1; i >= 0; i-- {
			wrapped = mws[i](wrapped)
		}
		return wrapped
	}
}

// LoggingMiddleware 记录 stage 的执行（开始、结束、耗时、错误）
func LoggingMiddleware(logger *log.Logger) Middleware {
	return func(next StageRunner) StageRunner {
		return func(rc *RunContext, st Stage) StageResult {
			nameTag := fmt.Sprintf("[%s]", st.Name())
			logger.Printf("%s Stage started (traceID=%s)", nameTag, rc.TraceID)

			start := time.Now()
			result := next(rc, st)
			elapsed := time.Since(start)

			if result.IsFailed() {
				logger.Printf("%s Stage failed after %v. Err: %v", nameTag, elapsed, result.Err)
			} else if result.Status == StageSkipped {
				logger.Printf("%s Stage skipped", nameTag)
			} else {
				logger.Printf("%s Stage succeeded in %v", nameTag, elapsed)
			}

			if result.Metrics == nil {
				result.Metrics = make(map[string]float64)
			}
			result.Metrics["duration_ms"] = float64(elapsed.Milliseconds())

			return result
		}
	}
}

// RecoveryMiddleware 防止 panic，将其转换为 StageFailed 错误
func RecoveryMiddleware(logger *log.Logger) Middleware {
	return func(next StageRunner) StageRunner {
		return func(rc *RunContext, st Stage) (result StageResult) {
			defer func() {
				if r := recover(); r != nil {
					logger.Printf("[%s] Stage panicked: %v", st.Name(), r)
					result = StageResult{
						Status: StageFailed,
						Err:    fmt.Errorf("stage panic: %v", r),
					}
				}
			}()

			result = next(rc, st)
			return result
		}
	}
}

// MetricsMiddleware 统计 stage 的执行指标（调用次数、失败次数等）
// 这里简单示例，实现中可以集成到监控系统
func MetricsMiddleware(metricsCollector map[string]map[string]float64) Middleware {
	return func(next StageRunner) StageRunner {
		return func(rc *RunContext, st Stage) StageResult {
			stageName := st.Name()
			if metricsCollector[stageName] == nil {
				metricsCollector[stageName] = make(map[string]float64)
			}

			metrics := metricsCollector[stageName]
			metrics["total_calls"]++

			result := next(rc, st)

			if result.IsFailed() {
				metrics["total_failures"]++
			} else if result.Status == StageSuccess {
				metrics["total_success"]++
			}

			return result
		}
	}
}

// TimeoutMiddleware 给 stage 增加超时限制（可选）
// 如果 stage 执行超过指定时间则返回失败
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next StageRunner) StageRunner {
		return func(rc *RunContext, st Stage) StageResult {
			// 创建带超时的子上下文
			childRc, cancel := rc.WithCancel()
			defer cancel()

			// 用 done 通道来等待执行完成
			done := make(chan StageResult, 1)
			go func() {
				done <- next(childRc, st)
			}()

			timer := time.NewTimer(timeout)
			defer timer.Stop()

			select {
			case result := <-done:
				return result
			case <-timer.C:
				cancel()
				return StageResult{
					Status: StageFailed,
					Err:    fmt.Errorf("stage timeout after %v", timeout),
				}
			}
		}
	}
}

// RetryMiddleware 根据 ErrorPolicy 自动重试失败的 stage
func RetryMiddleware(policy ErrorPolicy) Middleware {
	return func(next StageRunner) StageRunner {
		return func(rc *RunContext, st Stage) StageResult {
			var result StageResult
			attempt := 0

			for {
				result = next(rc, st)

				if result.IsSuccess() {
					return result
				}

				if !policy.ShouldRetry(st.Name(), result.Err, attempt) {
					return result
				}

				attempt++
				// 这里可以加回退策略（如指数退避），当前简单实现不等待
			}
		}
	}
}
