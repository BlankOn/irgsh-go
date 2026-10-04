// Package cancel carries job cancellation requests from chief to the workers.
//
// Chief marks a job cancelled in Redis and announces it on a pub/sub channel.
// A worker already running that job reacts at once; a worker that only picks
// the job up later still finds the mark and refuses to start it. The mark is
// what makes a job that is merely queued cancellable at all - machinery has no
// way to withdraw a task once it has been sent.
//
// Two Redis keys are involved:
//
//	irgsh:cancel:<taskUUID>   the mark, read by a worker as it starts a job
//	irgsh:cancel:live         pub/sub channel of task UUIDs, for running jobs
package cancel

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	keyPrefix = "irgsh:cancel:"

	// Channel is the pub/sub channel announcing cancelled task UUIDs.
	Channel = "irgsh:cancel:live"

	// MarkTTL is how long a cancellation mark survives. It only has to
	// outlive the queued job it applies to.
	MarkTTL = 7 * 24 * time.Hour

	// checkTimeout bounds the mark lookup a worker does as it starts a job:
	// an unreachable Redis must not hold up the job itself.
	checkTimeout = 5 * time.Second
)

// MarkKey returns the Redis key holding the cancellation mark of a job.
func MarkKey(taskUUID string) string { return keyPrefix + taskUUID }

func newClient(redisURL string) (*redis.Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}
	client := redis.NewClient(opt)
	if err := client.Ping(context.Background()).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}
	return client, nil
}

// Requester is the chief side: it records and announces cancellations.
type Requester struct {
	client *redis.Client
}

func NewRequester(redisURL string) (*Requester, error) {
	client, err := newClient(redisURL)
	if err != nil {
		return nil, err
	}
	return &Requester{client: client}, nil
}

func (r *Requester) Close() error { return r.client.Close() }

// Request marks the job cancelled and announces it to the workers.
func (r *Requester) Request(ctx context.Context, taskUUID string) error {
	pipe := r.client.Pipeline()
	pipe.Set(ctx, MarkKey(taskUUID), time.Now().UTC().Format(time.RFC3339), MarkTTL)
	pipe.Publish(ctx, Channel, taskUUID)
	_, err := pipe.Exec(ctx)
	return err
}

// IsRequested reports whether the job carries a cancellation mark.
func (r *Requester) IsRequested(ctx context.Context, taskUUID string) (bool, error) {
	n, err := r.client.Exists(ctx, MarkKey(taskUUID)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Watcher is the worker side: it holds one subscription for the process and
// hands out a Job per task it is running.
type Watcher struct {
	client *redis.Client
	pubsub *redis.PubSub
	stop   context.CancelFunc

	mu   sync.Mutex
	jobs map[string][]*Job
}

// NewWatcher subscribes to the cancellation channel. A worker should treat a
// failure as non-fatal and keep the nil *Watcher: Guard still works on it, the
// jobs it hands out simply never get cancelled.
func NewWatcher(redisURL string) (*Watcher, error) {
	client, err := newClient(redisURL)
	if err != nil {
		return nil, err
	}

	ctx, stop := context.WithCancel(context.Background())
	pubsub := client.Subscribe(ctx, Channel)
	if _, err := pubsub.Receive(ctx); err != nil {
		stop()
		pubsub.Close()
		client.Close()
		return nil, fmt.Errorf("failed to subscribe to the cancellation channel: %w", err)
	}

	w := &Watcher{client: client, pubsub: pubsub, stop: stop, jobs: map[string][]*Job{}}
	go w.listen(ctx)
	return w, nil
}

func (w *Watcher) Close() error {
	if w == nil {
		return nil
	}
	w.stop()
	w.pubsub.Close()
	return w.client.Close()
}

func (w *Watcher) listen(ctx context.Context) {
	ch := w.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			w.deliver(msg.Payload)
		}
	}
}

func (w *Watcher) deliver(taskUUID string) {
	w.mu.Lock()
	jobs := append([]*Job(nil), w.jobs[taskUUID]...)
	w.mu.Unlock()
	for _, job := range jobs {
		job.markRequested()
	}
}

// Guard starts watching taskUUID. The returned Job is never nil, including on
// a nil *Watcher, so a worker that could not reach Redis needs no branches of
// its own: its jobs simply never get cancelled.
//
// Callers must Release the job when it ends.
func (w *Watcher) Guard(taskUUID string) *Job {
	job := w.register(taskUUID)
	if w == nil || w.client == nil {
		return job
	}

	// A job cancelled while it was still queued has no announcement left to
	// hear: the mark is the only record of it. Registering first means an
	// announcement arriving during this lookup is not missed either.
	checkCtx, done := context.WithTimeout(context.Background(), checkTimeout)
	defer done()
	if n, err := w.client.Exists(checkCtx, MarkKey(taskUUID)).Result(); err != nil {
		log.Printf("cancel: unable to read the cancellation mark of %s: %v\n", taskUUID, err)
	} else if n > 0 {
		job.markRequested()
	}

	return job
}

// register creates the job handle and, unless the watcher is nil, subscribes
// it to the announcements for that task UUID.
func (w *Watcher) register(taskUUID string) *Job {
	ctx, cancel := context.WithCancel(context.Background())
	job := &Job{taskUUID: taskUUID, watcher: w, ctx: ctx, cancel: cancel}
	if w == nil {
		return job
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.jobs[taskUUID] = append(w.jobs[taskUUID], job)
	return job
}

// Job is one running task's view of cancellation.
type Job struct {
	taskUUID string
	watcher  *Watcher
	ctx      context.Context
	cancel   context.CancelFunc

	mu              sync.Mutex
	requested       bool
	uninterruptible bool
}

// Context is cancelled when the job is cancelled. Pass it to every command the
// job runs so they stop with it.
func (j *Job) Context() context.Context { return j.ctx }

// Requested reports whether cancellation has been asked for.
func (j *Job) Requested() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.requested
}

// Uninterruptible stops cancellation from interrupting the job from here on.
// The request is still recorded - Requested keeps reporting it - only the
// context is left alone, so the work in progress runs to its end.
//
// The repo worker uses this around reprepro: interrupting reprepro, an export
// above all, can leave its database corrupted, which costs far more than
// letting one unwanted injection finish.
func (j *Job) Uninterruptible() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.uninterruptible = true
}

func (j *Job) markRequested() {
	j.mu.Lock()
	j.requested = true
	uninterruptible := j.uninterruptible
	j.mu.Unlock()

	if !uninterruptible {
		j.cancel()
	}
}

// Release stops watching the job and releases its context.
func (j *Job) Release() {
	j.cancel()
	if j.watcher == nil {
		return
	}

	w := j.watcher
	w.mu.Lock()
	defer w.mu.Unlock()
	jobs := w.jobs[j.taskUUID]
	for i, other := range jobs {
		if other == j {
			w.jobs[j.taskUUID] = append(jobs[:i], jobs[i+1:]...)
			break
		}
	}
	if len(w.jobs[j.taskUUID]) == 0 {
		delete(w.jobs, j.taskUUID)
	}
}
