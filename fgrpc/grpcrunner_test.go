// Copyright 2017 Istio Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

// Adapted from istio/proxy/test/backend/echo with error handling and
// concurrency fixes and making it as low overhead as possible
// (no std output by default)

package fgrpc

import (
	"fmt"
	"testing"
	"time"

	"istio.io/fortio/log"
	"istio.io/fortio/periodic"

	"google.golang.org/grpc/health/grpc_health_v1"
)

var (
	// Generated from "make cert"
	caCrt  = "../cert-tmp/ca.crt"
	svrCrt = "../cert-tmp/server.crt"
	svrKey = "../cert-tmp/server.key"
)

func TestGRPCRunner(t *testing.T) {
	log.SetLogLevel(log.Info)
	iPort := PingServer("0", "", "", "bar", 0)
	iDest := fmt.Sprintf("localhost:%d", iPort)
	sPort := PingServer("0", svrCrt, svrKey, "bar", 0)
	sDest := fmt.Sprintf("localhost:%d", sPort)

	tests := []struct {
		name       string
		runnerOpts GRPCRunnerOptions
		expect     bool
	}{
		{
			name: "insecure runner",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:        100,
					Resolution: 0.00001,
				},
				Destination: iDest,
				Profiler:    "test.profile",
			},
			expect: true,
		},
		{
			name: "secure runner",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:        100,
					Resolution: 0.00001,
				},
				Destination: sDest,
				CACert:      caCrt,
				Profiler:    "test.profile",
			},
			expect: true,
		},
		{
			name: "invalid insecure runner to secure server",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:        100,
					Resolution: 0.00001,
				},
				Destination: sDest,
				Profiler:    "test.profile",
			},
			expect: false,
		},
		{
			name: "valid runner using nil credentials to Internet https server",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:        100,
					Resolution: 0.00001,
				},
				Destination: "https://fortio.istio.io:443",
				Profiler:    "test.profile",
			},
			expect: true,
		},
		{
			name: "invalid secure runner to insecure server",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:        100,
					Resolution: 0.00001,
				},
				Destination: iDest,
				CACert:      caCrt,
				Profiler:    "test.profile",
			},
			expect: false,
		},
		{
			name: "invalid runner using test cert to https prefix Internet server",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:        100,
					Resolution: 0.00001,
				},
				Destination: "https://fortio.istio.io:443",
				CACert:      caCrt,
				Profiler:    "test.profile",
			},
			expect: false,
		},
		{
			name: "invalid runner using test cert to no prefix Internet server",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:        100,
					Resolution: 0.00001,
				},
				Destination: "fortio.istio.io:443",
				CACert:      caCrt,
				Profiler:    "test.profile",
			},
			expect: false,
		},
		{
			name: "invalid name in runner cert",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:        100,
					Resolution: 0.00001,
				},
				Destination:  sDest,
				CACert:       caCrt,
				CertOverride: "invalidName",
				Profiler:     "test.profile",
			},
			expect: false,
		},
	}
	for _, test := range tests {
		res, err := RunGRPCTest(&test.runnerOpts)
		switch {
		case err != nil && test.expect:
			t.Errorf("Test case: %s failed due to unexpected error: %v", test.name, err)
			return
		case err == nil && !test.expect:
			t.Errorf("Test case: %s failed due to unexpected response: %v", test.name, res)
			return
		case err == nil && test.expect:
			totalReq := res.DurationHistogram.Count
			ok := res.RetCodes[grpc_health_v1.HealthCheckResponse_SERVING]
			if totalReq != ok {
				t.Errorf("Test case: %s failed. Mismatch between requests %d and ok %v",
					test.name, totalReq, res.RetCodes)
			}
		}
	}
}

func TestGRPCRunnerMaxStreams(t *testing.T) {
	log.SetLogLevel(log.Info)
	port := PingServer("0", "", "", "maxstream", 10)
	destination := fmt.Sprintf("localhost:%d", port)

	opts := GRPCRunnerOptions{
		RunnerOptions: periodic.RunnerOptions{
			QPS:        100,
			NumThreads: 1,
		},
		Destination: destination,
		Streams:     10, // will be batches of 10 max
		UsePing:     true,
		Delay:       20 * time.Millisecond,
	}
	o1 := opts
	res, err := RunGRPCTest(&o1)
	if err != nil {
		t.Error(err)
		return
	}
	totalReq := res.DurationHistogram.Count
	avg10 := res.DurationHistogram.Avg
	ok := res.RetCodes[grpc_health_v1.HealthCheckResponse_SERVING]
	if totalReq != ok {
		t.Errorf("Mismatch1 between requests %d and ok %v", totalReq, res.RetCodes)
	}
	if avg10 < opts.Delay.Seconds() || avg10 > 3*opts.Delay.Seconds() {
		t.Errorf("Ping delay not working, got %v for %v", avg10, opts.Delay)
	}
	o2 := opts
	o2.Streams = 20
	res, err = RunGRPCTest(&o2)
	if err != nil {
		t.Error(err)
		return
	}
	totalReq = res.DurationHistogram.Count
	avg20 := res.DurationHistogram.Avg
	ok = res.RetCodes[grpc_health_v1.HealthCheckResponse_SERVING]
	if totalReq != ok {
		t.Errorf("Mismatch2 between requests %d and ok %v", totalReq, res.RetCodes)
	}
	// Half of the calls should take 2x (delayed behind maxstreams)
	if avg20 < 1.5*opts.Delay.Seconds() {
		t.Errorf("Expecting much slower average with 20/10 %v %v", avg20, avg10)
	}
}

func TestGRPCRunnerWithError(t *testing.T) {
	log.SetLogLevel(log.Info)
	iPort := PingServer("0", "", "", "bar", 0)
	iDest := fmt.Sprintf("localhost:%d", iPort)
	sPort := PingServer("0", svrCrt, svrKey, "bar", 0)
	sDest := fmt.Sprintf("localhost:%d", sPort)

	tests := []struct {
		name       string
		runnerOpts GRPCRunnerOptions
	}{
		{
			name: "insecure runner",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:      10,
					Duration: 1 * time.Second,
				},
				Destination: iDest,
				Service:     "svc2",
			},
		},
		{
			name: "secure runner",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:      10,
					Duration: 1 * time.Second,
				},
				Destination: sDest,
				CACert:      caCrt,
				Service:     "svc2",
			},
		},
		{
			name: "invalid insecure runner to secure server",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:      10,
					Duration: 1 * time.Second,
				},
				Destination: sDest,
				Service:     "svc2",
			},
		},
		{
			name: "invalid secure runner to insecure server",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:      10,
					Duration: 1 * time.Second,
				},
				Destination: iDest,
				CACert:      caCrt,
				Service:     "svc2",
			},
		},
		{
			name: "invalid name in runner cert",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:      10,
					Duration: 1 * time.Second,
				},
				Destination:  sDest,
				CACert:       caCrt,
				CertOverride: "invalidName",
				Service:      "svc2",
			},
		},
		{
			name: "valid runner using nil credentials to Internet https server",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:      10,
					Duration: 1 * time.Second,
				},
				Destination: "https://fortio.istio.io:443",
				Service:     "svc2",
			},
		},
		{
			name: "invalid runner using test cert to https prefix Internet server",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:      10,
					Duration: 1 * time.Second,
				},
				Destination: "https://fortio.istio.io:443",
				CACert:      caCrt,
				Service:     "svc2",
			},
		},
		{
			name: "invalid runner using test cert to no prefix Internet server",
			runnerOpts: GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:      10,
					Duration: 1 * time.Second,
				},
				Destination: "fortio.istio.io:443",
				CACert:      caCrt,
				Service:     "svc2",
			},
		},
	}
	for _, test := range tests {
		_, err := RunGRPCTest(&test.runnerOpts)
		if err == nil {
			t.Error("Was expecting initial error when connecting to secure without AllowInitialErrors")
		}
		test.runnerOpts.AllowInitialErrors = true
		res, err := RunGRPCTest(&test.runnerOpts)
		if err != nil {
			t.Errorf("Test case: %s failed due to unexpected error: %v", test.name, err)
			return
		}
		totalReq := res.DurationHistogram.Count
		numErrors := res.RetCodes[-1]
		if totalReq != numErrors {
			t.Errorf("Test case: %s failed. Mismatch between requests %d and errors %v",
				test.name, totalReq, res.RetCodes)
		}
	}
}

func TestGRPCDestination(t *testing.T) {
	tests := []struct {
		name   string
		dest   string
		output string
	}{
		{
			"valid hostname",
			"localhost",
			"localhost:8079",
		},
		{
			"hostname and port",
			"localhost:1234",
			"localhost:1234",
		},
		{
			"hostname with http prefix",
			"http://localhost",
			"localhost:80",
		},
		{
			"Hostname with https prefix",
			"https://localhost",
			"localhost:443",
		},
		{
			"IPv4 address",
			"1.2.3.4",
			"1.2.3.4:8079",
		},
		{
			"IPv4 address and port",
			"1.2.3.4:5678",
			"1.2.3.4:5678",
		},
		{
			"IPv6 address",
			"2001:dba::1",
			"[2001:dba::1]:8079",
		},
		{
			"IPv6 address and port",
			"[2001:dba::1]:1234",
			"[2001:dba::1]:1234",
		},
		{
			"IPv6 address with http prefix",
			"http://2001:dba::1",
			"[2001:dba::1]:80",
		},
		{
			"IPv6 address with https prefix",
			"https://2001:dba::1",
			"[2001:dba::1]:443",
		},
	}

	for _, tc := range tests {
		dest := grpcDestination(tc.dest)
		if dest != tc.output {
			t.Errorf("Test case: %s failed to set gRPC destination\n\texpected: %s\n\t  actual: %s",
				tc.name,
				tc.output,
				dest,
			)
		}
	}
}
