package middlewares

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Simple in-memory rate limiter for demonstration purposes
// In production, use Redis or similar

type clientLimiter struct {
	tokens int
	lastUpdate time.Time
}

var (
	limiters = make(map[string]*clientLimiter)
	mu       sync.Mutex
	rate     = 5                 // 5 requests per second
	capacity = 10                // max 10 tokens
)

func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		mu.Lock()
		limiter, exists := limiters[clientIP]
		if !exists {
			limiter = &clientLimiter{
				tokens:     capacity,
				lastUpdate: time.Now(),
			}
			limiters[clientIP] = limiter
		}

		// Replenish tokens
		now := time.Now()
		elapsed := now.Sub(limiter.lastUpdate).Seconds()
		limiter.tokens += int(elapsed * float64(rate))
		if limiter.tokens > capacity {
			limiter.tokens = capacity
		}
		limiter.lastUpdate = now

		if limiter.tokens <= 0 {
			mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests"})
			c.Abort()
			return
		}

		limiter.tokens--
		mu.Unlock()

		c.Next()
	}
}
