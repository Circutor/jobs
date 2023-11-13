# Jobs Package

This package provides an asynchronous job processing system built on top of the `github.com/hbhiken/async` library.

## Overview

- The package introduces a `ServerMux` type, which serves as a wrapper around `async.ServeMux`. It's responsible for handling various job kinds and their respective handlers.
- A `Client` type acts as a wrapper around the `async.Client` to enqueue tasks.
- Configuration options are available for the server, including setting up the maximum number of concurrent workers and priority of task queues.

## Key Components

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

## Basic Usage

For this basic usage, let's use the example of integrating jobs out of [the existing `RegisterUserUseCase`](https://gitlab.com/circutor/cloud/myc-cloud/-/blob/main/business/usecase/user/register.go#L12).

### Enqueing jobs - Example from the User Core (exceptionally from the UseCase)

Preferably we would enqueue jobs from the Core as for the existing rules:

- We do things in the specific Core to orchestrate within a domain.
- We do things in the Use Cases to orchestrate different domains. 

Jobs in the 99% of the cases will be a domain logic, exceptionally we could have a job, that has to orchestrate two different domains. Likely on those situations we can enqueue two parallel jobs, chaining jobs will cause coupling between domains.

#### Enqueing jobs - Initializing the UserCore with a job's client

This would be the updated Core definition from [the User Core source code](https://gitlab.com/circutor/cloud/myc-cloud/-/blob/main/business/core/user/user.go#L28):

```go
// business/core/user/user.go
// ...

// JobsEnqueuer defined the method to enable the User Core to execute user jobs.
type JobEnqueuer interface {
    Enqueue (jobs.Task) error
}

// UserCore user core layer struct.
type UserCore struct {
    userStore UserStore
}

// NewUserCore creates a user core layer.
func NewUserCore(userStore UserStore, jobEnqueuer JobEnqueuer) UserCore {
	return UserCore{
            userStore: userStore,
            jobEnqueuer: jobEnqueuer,
	}
}
```

This is the application loader initializing the jobClient and using it on the User Core initialization.

```go
// app/services/myc-api/main.go
// ...

userStore := userDB.NewUserStore(DB)
jobsClient := jobs.NewClient(cfg.RedisURL)
defer jobsClient.Close()

userCore := userCore.NewUserCore(userStore, jobsClient)
```

#### Enqueing jobs - Using the client to enqueue a job

```go
// business/core/user/user.go
// ...
 
func (core UserCore) Create(ctx context.Context, user entities.User, creationType string) (entities.User, error) {
    // ...
    // TODO: The job type and the payload should be defined in a common place with real constant and types.
    createUserPayload, err := json.Marshal(user)
    if err != nil {
        return fmt.Errorf("json.Marshal, err: %w", err)
    }

    task := jobs.NewTask(userJobs.CreateUserTaskType, createUserPayload)
    if err := client.Enqueue(task); err != nil {
        return fmt.Errorf("client.Enqueue, %w", err)
    }
    // ...
}
```

### Running jobs - Executing jobs

The following is the job definition, very much likely every handler, we need to define the dependencies for every job.

```go
// TODO - This path doesn't exists yet, it's an job's infra equivalent to handlers.
// app/services/myc-api/jobs/v1/user/create.go

// CreateUserTaskType is the Job Type for triggering the CreateUser Job.
const CreateUserTaskType = "createUser"

// CreateUser define the dependencies to create user asynchronously by jobs.
type CreateUser struct {
    userStore: userStore
}

// NewCreateUser initializes a CreateUser job.
func NewCreateUser(userStore UserStore) CreateUser {
    return CreateUser {
        userStore: userStore,
    }
}

// ProcessTask is the create user asychornous execution method.
func (cu *CreateUser) ProcessTask (ctx context.Context, task *jobs.Task) error {
    // create user code...
}
```

The following snippet is the job server initialization.

```go
// TODO - This would likely be a new main.
}
server := jobs.NewServer(cfg.RedisURL, 
    jobs.WithConfig(
        jobs.Config{
            Concurrency: 10,
            Queues: map[string]int{
                "critical": 6,
                "default":  3,
                "low":      1,
            },
        },
    ),
)

userStore := userDB.NewUserStore(DB)
createUserJob := userJobs.NewCreateUser(userStore)

server.HandleFunc(userJobs.CreateUserTaskType, createUserJob.ProcessTask)

if err := server.Run(); err != nil {
    log.Fatalf("could not run server: %v", err)
}
```
