package libv2ray

import (
  "context"
  "errors"
  "fmt"
  "io"
  "net"
  "net/http"
  "strconv"
  "strings"
  "time"

  corenet "github.com/xtls/xray-core/common/net"
  "github.com/xtls/xray-core/common/serial"
  core "github.com/xtls/xray-core/core"
  corestats "github.com/xtls/xray-core/features/stats"
  coreserial "github.com/xtls/xray-core/infra/conf/serial"
)

// Constants for environment variables
const (
  defaultTimeoutSec    = 9
)


// QueryStats retrieves and resets traffic statistics for a specific outbound tag and direction
// Returns the accumulated traffic value and resets the counter to zero
// Returns 0 if the stats manager is not initialized or the counter doesn't exist
func (x *CoreController) QueryStats(tag string, direct string) int64 {
  if x.statsManager == nil {
    return 0
  }
  counter := x.statsManager.GetCounter(fmt.Sprintf("outbound>>>%s>>>traffic>>>%s", tag, direct))
  if counter == nil {
    return 0
  }
  return counter.Set(0)
}

// QueryAllOutboundTrafficStats retrieves and resets all outbound traffic counters.
// Returns a single-line text in format: tag,direction,value;tag,direction,value;
// Returns an empty string if the stats manager is not initialized or no counters exist.
func (x *CoreController) QueryAllOutboundTrafficStats() string {
  if x.statsManager == nil {
    return ""
  }

  var b strings.Builder

  x.statsManager.VisitCounters(func(name string, counter corestats.Counter) bool {
    parts := strings.Split(name, ">>>")
    if len(parts) != 4 || parts[0] != "outbound" || parts[2] != "traffic" {
      return true
    }

    tag := parts[1]
    direct := parts[3]
    value := counter.Set(0)
    if value <= 0 {
      return true
    }

    b.WriteString(tag)
    b.WriteByte(',')
    b.WriteString(direct)
    b.WriteByte(',')
    b.WriteString(strconv.FormatInt(value, 10))
    b.WriteByte(';')
    return true
  })
  return b.String()
}

// MeasureDelay measures network latency to a specified URL through the current core instance
// Uses a 12-second timeout context and returns the round-trip time in milliseconds
// An error is returned if the connection fails or returns an unexpected status
func (x *CoreController) MeasureDelay(url string) (int64, error) {
  return x.MeasureDelayTo(url, defaultTimeoutSec)
}

// MeasureDelay measures network latency to a specified URL through the current core instance
// Uses a 12-second timeout context and returns the round-trip time in milliseconds
// An error is returned if the connection fails or returns an unexpected status
func (x *CoreController) MeasureDelayTo(url string, timeoutSeconds int32) (int64, error) {
  timeout := time.Duration(timeoutSeconds) * time.Second
  ctx, cancel := context.WithTimeout(context.Background(), timeout)
  defer cancel()

  return measureInstDelay(ctx, x.coreInstance, url, timeout)
}

// MeasureOutboundDelay measures the outbound delay for a given configuration and URL
func MeasureOutboundDelay(ConfigureFileContent string, url string) (int64, error) {
  return MeasureOutboundDelayTo(ConfigureFileContent, url, defaultTimeoutSec)
}

// MeasureOutboundDelayTo measures the outbound delay for a given configuration, URL and timeout
func MeasureOutboundDelayTo(ConfigureFileContent string, url string, timeoutSeconds int32) (int64, error) {
  config, err := coreserial.LoadJSONConfig(strings.NewReader(ConfigureFileContent))
  if err != nil {
    return -1, fmt.Errorf("config load error: %w", err)
  }

  // Simplify config for testing
  config.Inbound = nil
  var essentialApp []*serial.TypedMessage
  for _, app := range config.App {
    if app.Type == "xray.app.proxyman.OutboundConfig" ||
      app.Type == "xray.app.dispatcher.Config" ||
      app.Type == "xray.app.log.Config" {
      essentialApp = append(essentialApp, app)
    }
  }
  config.App = essentialApp

  inst, err := core.New(config)
  if err != nil {
    return -1, fmt.Errorf("instance creation failed: %w", err)
  }

  if err := inst.Start(); err != nil {
    return -1, fmt.Errorf("startup failed: %w", err)
  }
  defer inst.Close()
  return measureInstDelay(context.Background(), inst, url, time.Duration(timeoutSeconds)*time.Second)
}
func measureInstDelay(ctx context.Context, inst *core.Instance, url string, timeout time.Duration) (int64, error) {
  if inst == nil {
    return -1, errors.New("core instance is nil")
  }

  tr := &http.Transport{
    TLSHandshakeTimeout: timeout,
    DisableKeepAlives:   false,
    DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
      dest, err := corenet.ParseDestination(fmt.Sprintf("%s:%s", network, addr))
      if err != nil {
        return nil, err
      }
      return core.Dial(ctx, inst, dest)
    },
  }

  client := &http.Client{
    Transport: tr,
    Timeout:   timeout,
  }

  if url == "" {
    url = "https://www.google.com/generate_204"
  }

  req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
  if err != nil {
    return -1, fmt.Errorf("failed to create HTTP request: %w", err)
  }

  var minDuration int64 = -1
  success := false
  var lastErr error

  // Add exception handling and increase retry attempts
  const attempts = 2
  for i := 0; i < attempts; i++ {
    select {
    case <-ctx.Done():
      // Return immediately when context is canceled
      if !success {
        return -1, ctx.Err()
      }
      return minDuration, nil
    default:
      // Continue execution
    }

    start := time.Now()
    resp, err := client.Do(req)
    if err != nil {
      lastErr = err
      continue
    }

    // Ensure response body is closed
    defer func(resp *http.Response) {
      if resp != nil && resp.Body != nil {
        resp.Body.Close()
      }
    }(resp)

    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
      lastErr = fmt.Errorf("invalid status: %s", resp.Status)
      continue
    }

    // Handle possible errors when reading response body
    if _, err := io.Copy(io.Discard, resp.Body); err != nil {
      lastErr = fmt.Errorf("failed to read response body: %w", err)
      continue
    }

    duration := time.Since(start).Milliseconds()
    if !success || duration < minDuration {
      minDuration = duration
    }

    success = true
  }
  if !success {
    return -1, lastErr
  }
  return minDuration, nil
}