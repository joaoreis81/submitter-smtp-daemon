# Performance and Concurrency Analysis

## Current Architecture

### ✅ What IS Concurrent (Good)

1. **Goroutine-per-connection model**
   - The `go-smtp` library creates a new goroutine for each incoming connection
   - Multiple clients can connect simultaneously
   - Each connection is handled independently in parallel

2. **Non-blocking I/O**
   - Go's network stack uses non-blocking I/O
   - Efficient handling of many concurrent connections

3. **Thread-safe components**
   - `sync.RWMutex` in TLS manager for certificate reloads
   - `sync.RWMutex` in rate limiter for concurrent access
   - Atomic operations in metrics

4. **Independent session handling**
   - Each SMTP session (`Session` struct) is isolated
   - No shared state between sessions
   - No global locks during message processing

### ⚠️ Potential Bottlenecks

1. **Backend Connection per Session**
   - Each authentication creates a NEW backend connection
   - Connection is kept open until session ends
   - **Impact:** High backend connection count under load
   - **Mitigation:** Backend must support many concurrent connections

2. **Sequential Processing per Session**
   - Within a single session, commands are processed sequentially
   - AUTH → MAIL → RCPT → DATA is linear
   - **Impact:** None - this is SMTP protocol requirement

3. **No Connection Pooling**
   - Config has `pool_enabled: false` (not implemented)
   - Backend connections not reused across sessions
   - **Impact:** Higher latency, more backend load

4. **Rate Limiter Lock Contention**
   - `sync.RWMutex` on every auth attempt
   - Map operations with lock held
   - **Impact:** Minor - only during auth phase

5. **Logger Synchronization**
   - `slog.Logger` has internal locking
   - All logs are synchronous
   - **Impact:** Minor - logs are fast

### ❌ What is NOT Concurrent

1. **No Multi-Process Support**
   - Single process only
   - Cannot utilize multiple CPU cores effectively for CPU-bound tasks
   - **But:** SMTP is I/O-bound, not CPU-bound

2. **No Load Balancing**
   - No built-in multi-instance support
   - Requires external load balancer (HAProxy, Nginx, etc.)

3. **No Connection Pooling**
   - Feature defined but not implemented
   - Each session = 1 backend connection

## Performance Characteristics

### Expected Throughput

**Conservative estimate (without connection pooling):**
- **Concurrent connections:** 1,000+ (limited by OS file descriptors)
- **Auth throughput:** ~100-200 auth/sec (limited by backend)
- **Message throughput:** ~50-100 msg/sec (limited by backend relay)

**Bottlenecks:**
1. Backend authentication response time
2. Backend message relay response time
3. Network latency to backend
4. OS file descriptor limits

### Tested Limits

**Config defaults:**
```yaml
limits:
  max_connections: 1000       # NOT ENFORCED (feature not implemented)
  max_connections_per_ip: 10  # NOT ENFORCED (feature not implemented)
  read_timeout: 120s          # Works
  write_timeout: 120s         # Works
  idle_timeout: 300s          # NOT USED (bug - not connected)
```

## Scalability

### ✅ Scales Well For:

1. **I/O-bound workload** (SMTP is primarily I/O)
2. **Many concurrent connections** (goroutines are cheap)
3. **Low to medium message rates** (100-500 msg/sec)

### ⚠️ Limitations:

1. **Backend becomes bottleneck** at high load
   - Each auth needs backend round-trip
   - Each message needs backend relay
   - Solution: Fast backend or connection pooling

2. **No horizontal scaling** (single process)
   - Solution: Run multiple instances behind load balancer

3. **No connection limits enforced**
   - Config exists but not implemented
   - Solution: Use OS limits or implement feature

## Load Testing Recommendations

### Test Scenarios

1. **Concurrent Connections Test**
   ```bash
   # Open 100 connections simultaneously
   for i in {1..100}; do
     (python3 test-email.sh &)
   done
   ```

2. **Sustained Load Test**
   ```bash
   # Send messages continuously for 5 minutes
   ab -n 1000 -c 10 -t 300 smtp://localhost:587/
   ```

3. **Backend Failure Test**
   - Stop backend server
   - Verify graceful degradation
   - Check error handling

### Performance Tuning

1. **OS-level:**
   ```bash
   # Increase file descriptor limit
   ulimit -n 65535
   
   # TCP tuning
   sysctl -w net.core.somaxconn=4096
   sysctl -w net.ipv4.tcp_max_syn_backlog=4096
   ```

2. **Config-level:**
   ```yaml
   limits:
     read_timeout: 30s    # Shorter timeout for faster failure
     write_timeout: 30s
     idle_timeout: 60s    # Disconnect idle clients
   
   backend:
     timeout: 5s          # Faster backend timeout
   ```

3. **Deployment:**
   - Run multiple instances (e.g., 4 processes)
   - Use HAProxy/Nginx for load balancing
   - Monitor with Prometheus metrics

## Comparison with Other Architectures

### Current: Goroutine-per-connection

**Pros:**
- ✅ Simple, clean code
- ✅ Good for I/O-bound workload
- ✅ Scales to thousands of connections
- ✅ Low latency per connection

**Cons:**
- ❌ Each goroutine uses ~2KB memory
- ❌ 10,000 connections = ~20MB just for goroutines
- ❌ GC pressure with many goroutines

### Alternative: Worker Pool

```go
// NOT IMPLEMENTED - example only
pool := make(chan *smtp.Conn, 100)
for i := 0; i < workers; i++ {
    go worker(pool)
}
```

**Pros:**
- ✅ Limited number of goroutines
- ✅ Better GC behavior
- ✅ More predictable resource usage

**Cons:**
- ❌ More complex code
- ❌ Connection queuing overhead
- ❌ Not needed for SMTP (I/O-bound)

## Recommendations

### For Production Deployment

1. **Single Instance** (up to ~500 msg/sec):
   - Current architecture is fine
   - Monitor backend performance
   - Set OS file descriptor limits high

2. **High Load** (500+ msg/sec):
   - Deploy 4-8 instances
   - Load balance with HAProxy/Nginx
   - Implement connection pooling
   - Fast backend (SSD, close network proximity)

3. **Very High Load** (5000+ msg/sec):
   - Implement connection pooling to backend
   - Consider backend replication
   - Use dedicated database for rate limiting
   - Implement connection limiting feature

### Priority Improvements

**High Priority:**
1. Implement `idle_timeout` (currently not used)
2. Implement connection pooling (`pool_enabled: true`)
3. Implement `max_connections` enforcement

**Medium Priority:**
4. Add connection tracking metrics
5. Implement `max_connections_per_ip`
6. Add circuit breaker for backend failures

**Low Priority:**
7. Worker pool architecture (only if >10k connections needed)
8. Async logging (only if logging becomes bottleneck)

## Benchmark Results (Estimated)

### Single Instance (on 4-core server)

| Metric | Value |
|--------|-------|
| Concurrent connections | 1,000 |
| Auth throughput | 100-200/sec |
| Message throughput | 50-100/sec |
| Backend latency impact | 1:1 ratio |
| Memory usage | ~50-100MB |
| CPU usage | 10-30% |

### With Connection Pooling (if implemented)

| Metric | Value |
|--------|-------|
| Auth throughput | 500-1000/sec |
| Message throughput | 200-500/sec |
| Backend connections | 10-100 (pooled) |

## Conclusion

**Is it multi-processed?** 
- No - single process, multiple goroutines

**Does it support high load?**
- **Yes** for moderate load (100-500 msg/sec)
- **Requires scaling** for high load (500+ msg/sec)
- **Well-designed** for I/O-bound SMTP workload

**Key takeaway:** The architecture is sound for a proxy. The main bottleneck will be backend performance, not the proxy itself.
