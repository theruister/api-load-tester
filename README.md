# Usage

Either run with go run ./cmd/loadtest or with go build ./cmd/loadtest/laod.go and executing ./load

./load -url=http://localhost:8083/api/v1/users 

# Parameters

| parameter |type | default | description |
|-----------|-----|---------|-------------|
| url | string | "" | target URL (required)|
| method | string | GET | HTTP method (GET/POST/etc) | 
| body | string | "" | HTTP request body |
| rate | int | 50 | target requests per second | 
| workers | int | 10 | number of concurrent workers | 
| duration | int | 10 | how long to run the test in seconds |
| timeout | int | 5 | per-request timeout in seconds | 
| file | string | "" | config file to load (JSON format - example provided in config.json) |

# Design considerations 

## Parameter usage

Parameters are loaded in 3 stages allowing for ease of use
1. Default values are loaded 
2. If file is present update with any values defined in the config file
3. Override any previously set values with CLI parameters specifically set by the user

This 3 stage approach allows users to define tests in config files while also allowing some flexibility to override if a
one off test is desired. This flexibility allows users to make small changes to a test without having to fully edit
their json files. 

## Coordinated Omission 

In simpler load testers a routine will make a request, wait for response, then issues the next request. This results
in slower send rates when responses slow down, and causes latency measurements to seem better than they truly are. 

Here latency is calculated based off of when the request should have been sent. This is done via a dedicated goroutine
emitting 'send now' signals to workers. This maintains the rate at which requests should be sent if requests start to
slow down, providing trustworthy measurements under load.

i.e. If an endpoint can handle 200 connections simultaneously and 1000 users make requests each user will feel how
long it takes from when they initialize the request to when a response is, not the time it took a request to process 
after it is allowed a connection. 

# Example 

./load -file=conf.json
load testing http://localhost:8083/api/v1/users - 40 req/s target, 10 workers, for 20s

Results
-------
Total requests:   800 \
Successful:       800 \
Errors:           0 \
Duration:         20.001s \
Throughput:       40.0 req/s 

Latency \
  min: 849.643µs \
  avg: 2.703724ms \
  p50: 2.807727ms \
  p95: 3.317527ms \
  p99: 3.567663ms \
  max: 6.363071ms


# Notes

While writing unit tests for pool.go multiple real bugs were found in the initial implementation. 
1. No validation was done on the input parameters. This allowed for problems like 0 rate or 0 workers causing the load
   tester to fail. 
2. Invalid HTTP methods were allowed as parameters causing 100% failure rate. This wastes user time and resources and
   validating the method at the beginning allows for long test runs to be avoided if set up will fail. \
   2.a. This is now allowing for users to be case insensitive for method - All http requests are being with upper case
   methods to allow users less frustration. -method=get will work the same as -method=GET
