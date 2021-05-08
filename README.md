# spool

[![License MIT](https://img.shields.io/badge/License-MIT-blue.svg)](http://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/dc0d/spool.svg)](https://pkg.go.dev/github.com/dc0d/spool)
[![Go Report Card](https://goreportcard.com/badge/github.com/dc0d/spool)](https://goreportcard.com/report/github.com/dc0d/spool)


**spool** is a simple worker-pool for Go. Each job is a `func()` which is sent to the worker-pool. A worker will pick up the job from the mailbox and execute it.

First, initialize the worker-poll with a mailbox of a certain size:

```go
pool := Init(n)
defer pool.Stop()
```

The mailbox is just a `chan func()`. In fact the worker-pool itself is defined as:

```go
type Workerpool chan func()
```

Jobs can be sent to the worker-pool in two different manners, blocking and nonblocking. To send a job to the worker-pool and block until it's completed:

```go
pool.Blocking(func() {
    // ...
})
```

And to send a job to the worker-pool and then move on:

```go
pool.Nonblocking(func() {
    // ...
})
```

As long as there is an empty space in the mailbox, `Nonblocking` will just queue the job, and moves on. When there are no more empty spaces in the mailbox, `Nonblocking` becomes blocking.

A worker-pool by default has one worker. To add workers to the worker-pool:

```go
pool.Grow(9)
```

Now, the worker-pool has `10` workers, one default worker and nine added workers.

It's possible to add temporary workers to the worker-pool:

```go
pool.Grow(9, WithAbsoluteTimeout(time.Minute * 5))
```

Also, instead of using and absolute timeout, an idle timeout can be used. In this case, added workers will stop, if they are idle for a certain duration:

```go
pool.Grow(9, WithIdleTimeout(time.Minute * 5))
```

The `Blocking` and `Nonblocking` methods will panic if the worker-pool is stopped - to enforce visibility on job execution.

## spool serializes the jobs in single worker mode

- `TODO`
