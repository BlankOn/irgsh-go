// Package logstream carries worker log lines to chief in near real time.
//
// Workers publish each line of a job log to Redis, which every irgsh
// component already talks to. Chief subscribes and forwards the lines to the
// browser over SSE, so a running job can be watched before its log file is
// uploaded at the end of the job.
//
// Each job log has two Redis keys:
//
//	irgsh:logs:<taskUUID>:<logType>        capped list, backlog for late joiners
//	irgsh:logs:<taskUUID>:<logType>:live   pub/sub channel, live lines
//
// Both hold the same JSON encoded Message, so a subscriber can drop live
// messages it has already seen in the backlog by comparing sequence numbers.
package logstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/blankon/irgsh-go/pkg/systemutil"
	"github.com/go-redis/redis/v8"
)

const (
	keyPrefix = "irgsh:logs:"

	// MaxBacklogLines caps how many lines are retained for late joiners. A
	// verbose pbuilder run produces far more than this; the complete log is
	// still uploaded to chief when the job ends.
	MaxBacklogLines = 5000

	// BacklogTTL is how long the backlog of a job survives in Redis.
	BacklogTTL = 24 * time.Hour
)

// Message is one line of a job log.
type Message struct {
	// Seq is a per-job counter starting at 1, used to drop duplicates when a
	// subscriber reads the backlog and the live channel at the same time.
	Seq int64 `json:"seq"`

	Line string `json:"line"`

	// End marks the final message of a job. No further lines will follow.
	End bool `json:"end,omitempty"`
}

// BacklogKey returns the Redis key holding the retained lines of a job log.
func BacklogKey(taskUUID, logType string) string {
	return keyPrefix + taskUUID + ":" + logType
}

// ChannelKey returns the Redis pub/sub channel carrying live lines.
func ChannelKey(taskUUID, logType string) string {
	return BacklogKey(taskUUID, logType) + ":live"
}

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

// Publisher publishes job log lines. It is safe for concurrent use.
type Publisher struct {
	client *redis.Client
}

// NewPublisher connects to Redis. Callers should treat a failure as
// non-fatal: log streaming is an addition to the log file, not a replacement.
func NewPublisher(redisURL string) (*Publisher, error) {
	client, err := newClient(redisURL)
	if err != nil {
		return nil, err
	}
	return &Publisher{client: client}, nil
}

func (p *Publisher) Close() error {
	return p.client.Close()
}

// Stream returns a publisher bound to one job log. Only one worker writes a
// given job log, so the sequence counter can live in this process.
func (p *Publisher) Stream(taskUUID, logType string) *Stream {
	return &Stream{
		publisher: p,
		backlog:   BacklogKey(taskUUID, logType),
		channel:   ChannelKey(taskUUID, logType),
	}
}

// Stream publishes the lines of a single job log.
type Stream struct {
	publisher *Publisher
	backlog   string
	channel   string
	seq       atomic.Int64
}

// Reset discards any backlog left by an earlier run of the same job. Retries
// reuse the task UUID, and without this a viewer would replay the previous
// run and stop at its end marker instead of following the current one.
func (s *Stream) Reset(ctx context.Context) error {
	return s.publisher.client.Del(ctx, s.backlog).Err()
}

// Publish records one log line and delivers it to any live subscriber.
func (s *Stream) Publish(ctx context.Context, line string) error {
	return s.send(ctx, Message{Seq: s.seq.Add(1), Line: line})
}

// End marks the job log as finished so subscribers can close their streams
// instead of waiting for lines that will never arrive.
func (s *Stream) End(ctx context.Context) error {
	return s.send(ctx, Message{Seq: s.seq.Add(1), End: true})
}

func (s *Stream) send(ctx context.Context, msg Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	pipe := s.publisher.client.Pipeline()
	pipe.RPush(ctx, s.backlog, payload)
	pipe.LTrim(ctx, s.backlog, -MaxBacklogLines, -1)
	pipe.Expire(ctx, s.backlog, BacklogTTL)
	pipe.Publish(ctx, s.channel, payload)
	_, err = pipe.Exec(ctx)
	return err
}

// Subscriber reads job logs published by the workers.
type Subscriber struct {
	client *redis.Client
}

func NewSubscriber(redisURL string) (*Subscriber, error) {
	client, err := newClient(redisURL)
	if err != nil {
		return nil, err
	}
	return &Subscriber{client: client}, nil
}

func (s *Subscriber) Close() error {
	return s.client.Close()
}

// Follow streams a job log to onMessage: first the retained backlog, then
// live lines as they are published.
//
// It subscribes before reading the backlog so that no line published in
// between is lost; lines already present in the backlog are then dropped by
// sequence number.
//
// Follow returns when the job log ends, when ctx is cancelled, or when
// onMessage returns an error.
func (s *Subscriber) Follow(ctx context.Context, taskUUID, logType string, onMessage func(Message) error) error {
	pubsub := s.client.Subscribe(ctx, ChannelKey(taskUUID, logType))
	defer pubsub.Close()

	// Wait for the subscription to be established before reading the backlog.
	if _, err := pubsub.Receive(ctx); err != nil {
		return fmt.Errorf("failed to subscribe to log channel: %w", err)
	}

	backlog, err := s.client.LRange(ctx, BacklogKey(taskUUID, logType), 0, -1).Result()
	if err != nil {
		return fmt.Errorf("failed to read log backlog: %w", err)
	}

	var lastSeq int64
	for _, raw := range backlog {
		var msg Message
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		lastSeq = msg.Seq
		if err := onMessage(msg); err != nil {
			return err
		}
		if msg.End {
			return nil
		}
	}

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case received, ok := <-ch:
			if !ok {
				return nil
			}
			var msg Message
			if err := json.Unmarshal([]byte(received.Payload), &msg); err != nil {
				continue
			}
			// Already delivered from the backlog.
			if msg.Seq <= lastSeq {
				continue
			}
			lastSeq = msg.Seq
			if err := onMessage(msg); err != nil {
				return err
			}
			if msg.End {
				return nil
			}
		}
	}
}

// HasBacklog reports whether any lines are retained for a job log. Chief uses
// it to tell "job never streamed anything" apart from "job is streaming".
func (s *Subscriber) HasBacklog(ctx context.Context, taskUUID, logType string) (bool, error) {
	n, err := s.client.Exists(ctx, BacklogKey(taskUUID, logType)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// drainDelay gives the tailer time to pick up the last lines a job wrote
// before its stream is closed. Without it the closing lines of a job — the
// ones a watcher is usually waiting for — would often be missed.
const drainDelay = 750 * time.Millisecond

// Mirror starts tailing logPath, printing it to the worker's own stdout as
// before and publishing each line for live viewing in chief. The returned
// function drains the tailer and marks the stream finished; workers should
// defer it.
//
// A nil publisher (Redis unavailable at startup) degrades to the previous
// behaviour of only printing to stdout.
func Mirror(pub *Publisher, taskUUID, logType, logPath string) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())

	var stream *Stream
	if pub != nil {
		stream = pub.Stream(taskUUID, logType)
		if err := stream.Reset(context.Background()); err != nil {
			log.Printf("logstream: failed to reset log backlog for %s: %v\n", taskUUID, err)
		}
	}

	go systemutil.StreamLogContext(ctx, logPath, func(line string) {
		if stream == nil {
			return
		}
		if err := stream.Publish(ctx, line); err != nil && ctx.Err() == nil {
			log.Printf("logstream: failed to publish log line for %s: %v\n", taskUUID, err)
		}
	})

	return func() {
		time.Sleep(drainDelay)
		cancel()
		if stream != nil {
			if err := stream.End(context.Background()); err != nil {
				log.Printf("logstream: failed to close log stream for %s: %v\n", taskUUID, err)
			}
		}
	}
}
