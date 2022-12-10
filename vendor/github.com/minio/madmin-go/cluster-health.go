//
// MinIO Object Storage (c) 2022 MinIO, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package madmin

import (
	"context"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	minioWriteQuorumHeader     = "x-minio-write-quorum"
	minIOHealingDrives         = "x-minio-healing-drives"
	clusterCheckEndpoint       = "/minio/health/cluster"
	clusterReadCheckEndpoint   = "/minio/health/cluster/read"
	maintanenceURLParameterKey = "maintenance"
)

// HealthResult represents the cluster health result
type HealthResult struct {
	Healthy         bool
	MaintenanceMode bool
	WriteQuorum     int
	HealingDrives   int
}

// HealthOpts represents the input options for the health check
type HealthOpts struct {
	ClusterRead bool
	Maintenance bool
}

// Healthy will hit `/minio/health/cluster` and `/minio/health/cluster/ready` anonymous APIs to check the cluster health
func (an *AnonymousClient) Healthy(ctx context.Context, opts HealthOpts) (result HealthResult, err error) {
	if opts.ClusterRead {
		return an.clusterReadCheck(ctx)
	}
	return an.clusterCheck(ctx, opts.Maintenance)
}

func (an *AnonymousClient) clusterCheck(ctx context.Context, maintenance bool) (result HealthResult, err error) {
	urlValues := make(url.Values)
	if maintenance {
		urlValues.Set(maintanenceURLParameterKey, "true")
	}

	resp, err := an.executeMethod(ctx, http.MethodGet, requestData{
		relPath:     clusterCheckEndpoint,
		queryValues: urlValues,
	}, nil)
	defer closeResponse(resp)
	if err != nil {
		return result, err
	}

	if resp != nil {
		writeQuorumStr := resp.Header.Get(minioWriteQuorumHeader)
		if writeQuorumStr != "" {
			result.WriteQuorum, err = strconv.Atoi(writeQuorumStr)
			if err != nil {
				return result, err
			}
		}
		healingDrivesStr := resp.Header.Get(minIOHealingDrives)
		if healingDrivesStr != "" {
			result.HealingDrives, err = strconv.Atoi(healingDrivesStr)
			if err != nil {
				return result, err
			}
		}
		switch resp.StatusCode {
		case http.StatusOK:
			result.Healthy = true
		case http.StatusPreconditionFailed:
			result.MaintenanceMode = true
		default:
			// Not Healthy
		}
	}
	return result, nil
}

func (an *AnonymousClient) clusterReadCheck(ctx context.Context) (result HealthResult, err error) {
	resp, err := an.executeMethod(ctx, http.MethodGet, requestData{
		relPath: clusterReadCheckEndpoint,
	}, nil)
	defer closeResponse(resp)
	if err != nil {
		return result, err
	}

	if resp != nil {
		switch resp.StatusCode {
		case http.StatusOK:
			result.Healthy = true
		default:
			// Not Healthy
		}
	}
	return result, nil
}

// AliveOpts customizing liveness check.
type AliveOpts struct {
	Readiness bool // send request to /minio/health/ready
}

// AliveResult returns the time spent getting a response
// back from the server on /minio/health/live endpoint
type AliveResult struct {
	Endpoint       *url.URL      `json:"endpoint"`
	ResponseTime   time.Duration `json:"responseTime"`
	DNSResolveTime time.Duration `json:"dnsResolveTime"`
	Online         bool          `json:"online"` // captures x-minio-server-status
	Error          error         `json:"error"`
}

// Alive will hit `/minio/health/live` to check if server is reachable, optionally returns
// the amount of time spent getting a response back from the server.
func (an *AnonymousClient) Alive(ctx context.Context, opts AliveOpts, servers ...ServerProperties) (resultsCh chan AliveResult) {
	resource := "/minio/health/live"
	if opts.Readiness {
		resource = "/minio/health/ready"
	}

	scheme := "http"
	if an.endpointURL != nil {
		scheme = an.endpointURL.Scheme
	}

	resultsCh = make(chan AliveResult)
	go func() {
		defer close(resultsCh)
		if len(servers) == 0 {
			an.alive(ctx, an.endpointURL, resource, resultsCh)
		} else {
			var wg sync.WaitGroup
			wg.Add(len(servers))
			for _, server := range servers {
				server := server
				go func() {
					defer wg.Done()
					sscheme := server.Scheme
					if sscheme == "" {
						sscheme = scheme
					}
					u, err := url.Parse(sscheme + "://" + server.Endpoint)
					if err != nil {
						resultsCh <- AliveResult{
							Error: err,
						}
						return
					}
					an.alive(ctx, u, resource, resultsCh)
				}()
			}
			wg.Wait()
		}
	}()

	return resultsCh
}

func (an *AnonymousClient) alive(ctx context.Context, u *url.URL, resource string, resultsCh chan AliveResult) {
	var (
		dnsStartTime, dnsDoneTime   time.Time
		reqStartTime, firstByteTime time.Time
	)

	trace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStartTime = time.Now()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			dnsDoneTime = time.Now()
		},
		GetConn: func(_ string) {
			// GetConn is called again when trace is ON
			// https://github.com/golang/go/issues/44281
			if reqStartTime.IsZero() {
				reqStartTime = time.Now()
			}
		},
		GotFirstResponseByte: func() {
			firstByteTime = time.Now()
		},
	}

	resp, err := an.executeMethod(ctx, http.MethodGet, requestData{
		relPath:          resource,
		endpointOverride: u,
	}, trace)
	closeResponse(resp)
	var respTime time.Duration
	if firstByteTime.IsZero() {
		respTime = time.Since(reqStartTime)
	} else {
		respTime = firstByteTime.Sub(reqStartTime) - dnsDoneTime.Sub(dnsStartTime)
	}

	result := AliveResult{
		Endpoint:       u,
		ResponseTime:   respTime,
		DNSResolveTime: dnsDoneTime.Sub(dnsStartTime),
	}
	if err != nil {
		result.Error = err
	} else {
		result.Online = resp.StatusCode == http.StatusOK && resp.Header.Get("x-minio-server-status") != "offline"
	}

	select {
	case <-ctx.Done():
		return
	case resultsCh <- result:
	}
}
