## Jobs Package

This package provides an asynchronous job processing system built on top of the `github.com/hbhiken/async` library.

### Overview

- The package introduces a `ServerMux` type, which serves as a wrapper around `async.ServeMux`. It's responsible for handling various job kinds and their respective handlers.
- A `Client` type acts as a wrapper around the `async.Client` to enqueue tasks.
- Configuration options are available for the server, including setting up the maximum number of concurrent workers and priority of task queues.

### Key Components

1. **Task**:
    - Represents a task with a kind, payload, max retry attempts, and timeout.
    - Task options like `MaxRetry` and `Timeout` are available for customization.

2. **Server**:
    - `ServerMux` is the main component that wraps the `async.ServeMux`.
    - It allows registration of handlers using `HandleFunc`.
    - Jobs are processed asynchronously using the underlying async library.

3. **Client**:
    - Allows enqueuing tasks using the `Enqueue` method.
    - Can be closed using the `Close` method.

4. **Config**:
    - Allows customizing server options, including concurrency and queue priorities.
    - Provides a default configuration using `DefaultConfig`.

### Basic Usage

```go
const redisURL = "redis://127.0.0.1:6379"

// Initialize the server with default configurations
server := jobs.NewServer(redisURL)

// Register a handler for a job kind
server.HandleFunc("jobKind", func(ctx context.Context, task *jobs.Task) error {
    // Process the task here
    return nil
})

server.Run()

// Client Usage
client := jobs.NewClient()

task := jobs.NewTask("jobKind", payload)
client.Enqueue(task)

defer client.Close()
```

A more advanced usage:

```
package main

import (
	"context"
	"log"

    "gitlab.com/circutor/cloud/jobs"
)

const redisURL = "redis://127.0.0.1:6379"

func main() {
	config := jobs.Config{
		Concurrency: 10,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		},
	}

	server := jobs.NewServer(config)

	// Here's an example of how you might handle a job of type "sendEmail"
	server.HandleFunc("sendEmail", func(ctx context.Context, task *jobs.Task) error {
		// Here you can process the sendEmail task.
		// For simplicity, we're just printing the task's payload.
		log.Println(task.Payload)
		return nil
	})

	// Similarly, for a "resizeImage" task, you could have:
	server.HandleFunc("resizeImage", func(ctx context.Context, task *jobs.Task) error {
		// Here you can process the resizeImage task.
		log.Println(task.Payload)
		return nil
	})

	// Add more handlers as needed...

	client := jobs.NewClient(asynq.RedisClientOpt{Addr: redisURL})

	task := jobs.NewTask("sendEmail", []byte("example@email.com"))
	if err := client.Enqueue(task); err != nil {
		log.Fatalf("could not enqueue task: %v", err)
	}

	if err := server.Run(); err != nil {
		log.Fatalf("could not run server: %v", err)
	}
}
